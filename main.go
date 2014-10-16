package imgurpopular

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"gopkg.in/tweetlib.v2"

	"appengine"
	"appengine/memcache"
	"appengine/taskqueue"
	"appengine/urlfetch"
)

var (
	charactersReservedPerMedia int
	rateLimit                  = 180
	twitterClient              *tweetlib.Client
	twitterConfig              *tweetlib.Config
	twitterToken               *tweetlib.Token
	twitterTransport           *tweetlib.Transport
	imgurURL                   = "https://api.imgur.com/3/gallery/top/top/0.json"
)

var config struct {
	ImgurClientID            string `json:"imgurClientID"`
	ImgurClientSecret        string `json:"imgurClientSecret"`
	TwitterAPIKey            string `json:"twitterAPIKey"`
	TwitterAPISecret         string `json:"twitterAPISecret`
	TwitterAccessToken       string `json:"twitterAccessToken`
	TwitterAccessTokenSecret string `json:"twitterAccessTokenSecret"`
}

type image struct {
	Data []byte
}

// https://api.imgur.com/models/gallery_image
// https://api.imgur.com/models/gallery_album
type result struct {
	ID    string `json:"id"`
	Cover string `json:"cover,omitempty"`
	Link  string `json:"link,omitempty"`
	NSFW  bool   `json:"nsfw,omitempty"`
	Size  int    `json:"size,omitempty"`
	Title string `json:"title"`
}

type resultList []*result

type results struct {
	Data    resultList `json:"data"`
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

	http.Handle("/tasks/limits", appHandler(updateLimits))
	http.Handle("/tasks/poll", appHandler(pollImgur))
	http.Handle("/tasks/process", appHandler(processTasks))
}

func updateLimits(w http.ResponseWriter, r *http.Request) error {
	ctx := appengine.NewContext(r)

	client, err := buildTwitterClient(ctx)
	if err != nil {
		ctx.Errorf("Error creating tweetlib client: %s", err)
		return err
	}

	limits, err := client.Help.Limits()
	if err != nil {
		ctx.Errorf("Error getting Configuration: %s", err)
		return err
	}
	rateLimit = limits.ResourceFamilies["application"]["/application/rate_limit_status"].Remaining

	config, err := client.Help.Configuration()
	if err != nil {
		ctx.Errorf("Error getting Configuration: %s", err)
		return err
	}
	charactersReservedPerMedia = config.CharactersReservedPerMedia

	return nil
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
	rs := results{}
	err = decoder.Decode(&rs)
	if err != nil {
		ctx.Errorf("Error decoding Imgur results:", err)
		return err
	}

	if rs.Success != true || rs.Status != 200 {
		ctx.Errorf("unsuccessful request to imgur")
		return err
	}

	// Reverse the results since oldest is last
	for i := len(rs.Data) - 1; i > -1; i-- {
		r := rs.Data[i]
		if _, err := memcache.Get(ctx, r.ID); err == memcache.ErrCacheMiss {
			// not in cache--do nothing
		} else if err != nil {
			ctx.Errorf("error getting item with key %s: %v", r.ID, err)
			continue
		} else {
			// already in cache--skip
			continue
		}

		var data bytes.Buffer
		enc := json.NewEncoder(&data)
		err = enc.Encode(r)
		if err != nil {
			ctx.Errorf("Unable to encode result with ID %s", r.ID)
			continue
		}

		t := &taskqueue.Task{
			Name:    r.ID,
			Payload: data.Bytes(),
			Method:  "PULL",
			Tag:     strconv.FormatBool(r.NSFW),
		}
		if _, err := taskqueue.Add(ctx, t, "pull-queue"); err != nil && err != taskqueue.ErrTaskAlreadyAdded {
			ctx.Errorf("Unable to add task for ID %s: %s", r.ID, err)
			continue
		}
	}

	return nil
}

func buildTwitterClient(ctx appengine.Context) (client *tweetlib.Client, err error) {
	twitterConfig := &tweetlib.Config{
		ConsumerKey:    config.TwitterAPIKey,
		ConsumerSecret: config.TwitterAPISecret,
	}

	twitterToken := &tweetlib.Token{
		OAuthToken:  config.TwitterAccessToken,
		OAuthSecret: config.TwitterAccessTokenSecret,
	}

	twitterTransport := &tweetlib.Transport{
		Config:    twitterConfig,
		Token:     twitterToken,
		Transport: &urlfetch.Transport{Context: ctx, Deadline: 30 * time.Second},
	}

	return tweetlib.New(twitterTransport.Client())
}

func buildMedia(ctx appengine.Context, r *result) (media *tweetlib.TweetMedia, err error) {
	// ctx.Infof("Image size for %s: %d\n", r.ID, r.Size)
	// Twitter only allows 3 MB files for media
	// Sometimes the field is set, so assume too large
	if r.Size > 3000000 || r.Size == 0 {
		return nil, nil
	}

	var file string
	if r.Cover != "" {
		file = "http://i.imgur.com/" + r.Cover + ".jpg"
	} else {
		file = r.Link
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

	image, err := ioutil.ReadAll(response.Body)
	if err != nil {
		ctx.Errorf("Error reading response for file %s", file)
	}

	media = &tweetlib.TweetMedia{r.ID, image}

	return
}

func generateStatus(r *result, tooLarge bool) (status string) {
	titleLength := 105

	// Images can't be more than 3 MB on Twitter, so we shorten the title
	// a little bit to add room for the link to the image
	if tooLarge {
		// Make room for the amount of characters in a gallery link
		titleLength -= 31
	} else {
		// characters_reserved_per_media
		// https://dev.twitter.com/rest/reference/post/statuses/update_with_media
		titleLength -= charactersReservedPerMedia
	}

	title := r.Title
	if len(title) > titleLength {
		title = title[:titleLength-1] + "…"
	}
	var link string
	if r.Cover != "" {
		link = fmt.Sprintf("http://i.imgur.com/%s.jpg", r.Cover)
	} else {
		link = r.Link
	}

	status = title
	if tooLarge {
		status += fmt.Sprintf(" %s", link)
	}
	status += fmt.Sprintf(" (http://imgur.com/gallery/%s)", r.ID)

	return
}

func processTasks(w http.ResponseWriter, r *http.Request) error {
	// Just wait if we're getting close to being rate-limited by Twitter
	if rateLimit < 100 {
		return nil
	}

	ctx := appengine.NewContext(r)

	tasks, err := taskqueue.Lease(ctx, 20, "pull-queue", 30)
	if err != nil {
		ctx.Errorf("Unable to lease tasks from pull-queue: %s", err)
		return err
	}

	for _, task := range tasks {
		var r *result
		err = json.Unmarshal(task.Payload, &r)
		if err != nil {
			ctx.Errorf("Unable to decode result with ID %s: %s", task.Name, err)
			continue
		}

		media, err := buildMedia(ctx, r)
		if err != nil {
			ctx.Errorf(err.Error())
			continue
		}

		status := generateStatus(r, media == nil)

		_, err = postTweet(ctx, status, media, task.Tag)
		if err != nil {
			ctx.Errorf("Unable to post tweet [%s] %s: %s", task.Name, status, err)
			time.Sleep(15 * time.Minute)
			return err
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

func postTweet(ctx appengine.Context, status string, image *tweetlib.TweetMedia, sensitive string) (tweet *tweetlib.Tweet, err error) {
	twitterClient, err := buildTwitterClient(ctx)
	if err != nil {
		ctx.Errorf("Error creating tweetlib client: %s", err)
		return
	}

	opts := tweetlib.NewOptionals()
	opts.Add("possibly_sensitive", sensitive == "true")

	if image == nil {
		return twitterClient.Tweets.Update(status, nil)
	}

	return twitterClient.Tweets.UpdateWithMedia(status, image, nil)
}
