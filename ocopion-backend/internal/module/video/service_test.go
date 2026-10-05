package video

import (
	"errors"
	"reflect"
	"testing"

	"ocopion/internal/config"
)

type stubRepository struct {
	list []VideoConfig
	err  error
}

func (r stubRepository) GetVideoList() ([]VideoConfig, error) {
	return r.list, r.err
}

func TestVideoServiceGetVideoList(t *testing.T) {
	items := []VideoConfig{
		{FileName: "ios/clip.mp4", Cover: "ios/clip.png"},
		{FileName: "https://cdn.example.com/other.mp4", Cover: ""},
	}
	service := NewVideoService(stubRepository{list: items}, &config.Config{
		File: config.FileConfig{UploadViewBaseURL: " https://media.example.com/ "},
	})
	got, err := service.GetVideoList()
	if err != nil {
		t.Fatal(err)
	}
	want := []VideoConfig{
		{FileName: "https://media.example.com/ios/clip.mp4", Cover: "https://media.example.com/ios/clip.png"},
		{FileName: "https://cdn.example.com/other.mp4", Cover: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetVideoList() = %#v, want %#v", got, want)
	}
	if items[0].FileName != "ios/clip.mp4" {
		t.Fatal("repository list was modified")
	}
}

func TestVideoServiceGetVideoListWithoutBaseURL(t *testing.T) {
	items := []VideoConfig{{FileName: "clip.mp4", Cover: "clip.png"}}
	got, err := NewVideoService(stubRepository{list: items}, nil).GetVideoList()
	if err != nil || !reflect.DeepEqual(got, items) {
		t.Fatalf("GetVideoList() = %#v, %v", got, err)
	}
}

func TestVideoServiceGetVideoListRepositoryError(t *testing.T) {
	want := errors.New("repository failed")
	got, err := NewVideoService(stubRepository{err: want}, &config.Config{}).GetVideoList()
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("GetVideoList() = %#v, %v", got, err)
	}
}
