package imgurpopular

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"gopkg.in/tweetlib.v2"

	"appengine"
	"appengine/memcache"
	"appengine/taskqueue"
	"appengine/urlfetch"
)

var (
	twitterClient *tweetlib.Client
	imgurURL      = "https://api.imgur.com/3/gallery/top/top/0.json"
)

var config struct {
	ImgurClientID            string `json:"imgurClientID"`
	ImgurClientSecret        string `json:"imgurClientSecret"`
	TwitterAPIKey            string `json:"twitterAPIKey"`
	TwitterAPISecret         string `json:"twitterAPISecret`
	TwitterAccessToken       string `json:"twitterAccessToken`
	TwitterAccessTokenSecret string `json:"twitterAccessTokenSecret"`
}

type Image struct {
	ID   string `json:"id"`
	Link string `json:"link"`
}

// https://api.imgur.com/models/gallery_image
// https://api.imgur.com/models/gallery_album
type Result struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Cover string `json:"cover,omitempty"`
	Link  string `json:"link,omitempty"`
}

type ResultList []*Result

type Results struct {
	Data    ResultList `json:"data"`
	Success bool       `json:"success"`
	Status  int32      `json:"status"`
}

type appHandler func(http.ResponseWriter, *http.Request) error

func (fn appHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func init() {
	file, err := os.Open("./conf.json")
	if err != nil {
		log.Println("Could not open conf.json")
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Println("Error reading conf.json:", err)
	}

	http.Handle("/tasks/poll", appHandler(pollImgur))
	http.Handle("/tasks/process", appHandler(processTasks))
}

// Get popular images from Imgur
// https://api.imgur.com/endpoints/gallery
func pollImgur(w http.ResponseWriter, r *http.Request) error {
	ctx := appengine.NewContext(r)
	req, err := http.NewRequest("GET", imgurURL, nil)
	if err != nil {
		ctx.Errorf("Unable to create Imgur request:", err)
		return err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Client-ID %s", config.ImgurClientID))
	client := &http.Client{
		Transport: &urlfetch.Transport{Context: ctx, Deadline: 10 * time.Second},
	}
	res, err := client.Do(req)
	if err != nil {
		ctx.Errorf("Error fetching from Imgur: %s", err)
		return err
	}
	defer res.Body.Close()

	decoder := json.NewDecoder(res.Body)
	results := Results{}
	err = decoder.Decode(&results)
	if err != nil {
		ctx.Errorf("Error decoding Imgur results:", err)
		return err
	}

	if results.Success != true || results.Status != 200 {
		ctx.Errorf("unsuccessful request to imgur")
		return err
	}

	// Reverse the results since oldest is last
	for i := len(results.Data) - 1; i > -1; i-- {
		result := results.Data[i]
		// ctx.Infof("Processing result with ID %s", result.ID)
		if _, err := memcache.Get(ctx, result.ID); err == memcache.ErrCacheMiss {
			// not in cache--do nothing
		} else if err != nil {
			ctx.Errorf("error getting item with key %s: %v", result.ID, err)
			continue
		} else {
			// already in cache--skip
			continue
		}

		var data bytes.Buffer
		enc := json.NewEncoder(&data)
		err = enc.Encode(result)
		if err != nil {
			ctx.Errorf("Unable to encode result with ID %s", result.ID)
			continue
		}

		t := &taskqueue.Task{
			Name:    result.ID,
			Payload: data.Bytes(),
			Method:  "PULL",
		}
		if _, err := taskqueue.Add(ctx, t, "pull-queue"); err != nil && err != taskqueue.ErrTaskAlreadyAdded {
			ctx.Errorf("Unable to add task for ID %s: %s", result.ID, err)
			continue
		}
	}

	return nil
}

func downloadImage(ctx appengine.Context, result *Result) (image []byte, err error) {
	var file string
	if result.Cover != "" {
		file = "http://i.imgur.com/" + result.Cover + ".jpg"
	} else {
		file = result.Link
	}
	req, err := http.NewRequest("GET", file, nil)
	if err != nil {
		ctx.Errorf("Unable to create request: %s", err)
	}
	client := &http.Client{
		Transport: &urlfetch.Transport{Context: ctx, Deadline: 10 * time.Second},
	}
	response, err := client.Do(req)
	if err != nil {
		return
	}
	defer response.Body.Close()
	image, err = ioutil.ReadAll(response.Body)
	if err != nil {
		ctx.Errorf("Error reading response for file %s", file)
	}
	return
}

func generateStatus(result *Result, tooLarge bool) (status string) {
	titleLength := 105

	// Images can't be more than 3 MB, so we shorten the title a little bit
	// to add room for the link to the image
	if tooLarge {
		titleLength -= 31
	}

	title := result.Title
	if len(title) > titleLength {
		title = title[:titleLength-1] + "…"
	}
	var link string
	if result.Cover != "" {
		link = fmt.Sprintf("http://i.imgur.com/%s.jpg", result.Cover)
	} else {
		link = result.Link
	}

	status = title
	if tooLarge {
		status += fmt.Sprintf(" %s", link)
	}
	status += fmt.Sprintf(" (http://imgur.com/gallery/%s)", result.ID)

	return
}

func processTasks(w http.ResponseWriter, r *http.Request) error {
	ctx := appengine.NewContext(r)
	tasks, err := taskqueue.Lease(ctx, 20, "pull-queue", 30)
	if err != nil {
		ctx.Errorf("Unable to lease tasks from pull-queue: %s", err)
		return err
	}

	for _, task := range tasks {
		var result *Result
		err = json.Unmarshal(task.Payload, &result)
		if err != nil {
			ctx.Errorf("Unable to decode result with ID %s: %s", task.Name, err)
			continue
		}

		// image, err := downloadImage(ctx, result)
		// if err != nil {
		// 	ctx.Errorf(err.Error())
		// 	continue
		// }

		image := []byte{}
		media := &tweetlib.TweetMedia{result.ID + ".jpg", image}
		media = nil
		// tooLarge := false
		tooLarge := true
		if len(image) > 3000000 {
			media = nil
			tooLarge = true
		}

		status := generateStatus(result, tooLarge)

		_, err = postTweet(ctx, status, media)
		if err != nil {
			ctx.Errorf("Unable to post tweet %s", err)
			continue
		}

		item := &memcache.Item{
			Key:        task.Name,
			Expiration: 72 * time.Hour,
			Value:      []byte(""),
		}
		if err := memcache.Add(ctx, item); err != nil && err != memcache.ErrNotStored {
			ctx.Errorf("error adding item with ID %s: %s", task.Name, err)
			continue
		}

		err = taskqueue.Delete(ctx, task, "pull-queue")
		if err != nil {
			ctx.Errorf("Unable to delete task ID %s: %s", task.Name, err)
			continue
		}
	}
	return nil
}

func postTweet(ctx appengine.Context, status string, image *tweetlib.TweetMedia) (tweet *tweetlib.Tweet, err error) {
	tweetlibConfig := &tweetlib.Config{
		ConsumerKey:    config.TwitterAPIKey,
		ConsumerSecret: config.TwitterAPISecret,
	}

	token := &tweetlib.Token{
		OAuthToken:  config.TwitterAccessToken,
		OAuthSecret: config.TwitterAccessTokenSecret,
	}

	tr := &tweetlib.Transport{
		Config:    tweetlibConfig,
		Token:     token,
		Transport: &urlfetch.Transport{Context: ctx, Deadline: 10 * time.Second},
	}

	twitterClient, err = tweetlib.New(tr.Client())
	if err != nil {
		ctx.Errorf("Error creating tweetlib client: %s", err)
		return
	}
	if image == nil {
		return twitterClient.Tweets.Update(status, nil)
	} else {
		return twitterClient.Tweets.UpdateWithMedia(status, image, nil)
	}
}
