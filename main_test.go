package imgurpopular

import (
	"fmt"
	"testing"
)

var (
	id         = "abcd123"
	cover      = "123abcd"
	gallery    = fmt.Sprintf("http://imgur.com/gallery/%s", id)
	longTitle  = "Some really awesome image that I found the other day. It was so much amazingness."
	shortTitle = "Some really awesome image."
	cutTitle   = longTitle[0:73] + "…"
)

func TestGenerateStatus_AlbumCoverTooLarge_ShortTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", cover)
	r := &result{
		ID:    id,
		Cover: cover,
		Title: shortTitle,
	}

	expected := fmt.Sprintf("%s %s (%s)", r.Title, link, gallery)
	got := generateStatus(r, true)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_AlbumCoverTooLarge_LongTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", cover)
	r := &result{
		ID:    id,
		Cover: cover,
		Title: longTitle,
	}

	expected := fmt.Sprintf("%s %s (%s)", cutTitle, link, gallery)
	got := generateStatus(r, true)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_AlbumCover_LongTitle(t *testing.T) {
	r := &result{
		ID:    id,
		Cover: cover,
		Title: longTitle,
	}

	expected := fmt.Sprintf("%s (%s)", longTitle, gallery)
	got := generateStatus(r, false)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_AlbumLarge_LongTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", id)
	r := &result{
		ID:    id,
		Link:  link,
		Title: longTitle,
	}

	expected := fmt.Sprintf("%s %s (%s)", cutTitle, link, gallery)
	got := generateStatus(r, true)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_AlbumLarge_ShortTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", id)
	r := &result{
		ID:    id,
		Link:  link,
		Title: shortTitle,
	}

	expected := fmt.Sprintf("%s %s (%s)", shortTitle, link, gallery)
	got := generateStatus(r, true)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_AlbumSmall_LongTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", id)
	r := &result{
		ID:    id,
		Link:  link,
		Title: longTitle,
	}

	expected := fmt.Sprintf("%s (%s)", longTitle, gallery)
	got := generateStatus(r, false)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_AlbumSmall_ShortTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", id)
	r := &result{
		ID:    id,
		Link:  link,
		Title: shortTitle,
	}

	expected := fmt.Sprintf("%s (%s)", shortTitle, gallery)
	got := generateStatus(r, false)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}

func TestGenerateStatus_ImageLarge_LongTitle(t *testing.T) {
	link := fmt.Sprintf("http://i.imgur.com/%s.jpg", id)
	r := &result{
		ID:    id,
		Link:  link,
		Title: longTitle,
	}

	expected := fmt.Sprintf("%s %s (%s)", cutTitle, link, gallery)
	got := generateStatus(r, true)

	if len(got) > 142 {
		t.Errorf("Title is longer than 140 characters: %d", len(got))
	}

	if got != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, got)
	}
}
