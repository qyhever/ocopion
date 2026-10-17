package cloudflare

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUploadAfterSave(t *testing.T) {
	repo := mockRepo(t, successScript)
	writeFile(t, filepath.Join(filepath.Dir(repo.binPath), "gate"), "", 0644)
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/upload" {
			t.Error("wrong upload method/path")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("secret") != "test-secret" {
			t.Error("wrong secret")
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer file.Close()
		uploaded, err := io.ReadAll(file)
		if err != nil {
			t.Error(err)
		}
		saved, err := os.ReadFile(repo.clashPath)
		if err != nil || !bytes.Equal(saved, uploaded) || header.Filename != filepath.Base(repo.clashPath) {
			t.Error("uploaded file does not match saved file")
		}
		ips, err := os.ReadFile(repo.outputPath)
		if err != nil || string(ips) != strings.Join(testIPs(), "\n")+"\n" {
			t.Error("IP file not saved before upload")
		}
		close(entered)
		<-release
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	repo.uploadURL, repo.uploadSecret = server.URL+"/upload", "test-secret"
	service := NewService(repo)
	service.Start()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("upload not started")
	}
	state := service.Get()
	close(release)
	if state.Status != "running" || state.FinishedAt != nil {
		t.Fatal("task finished before upload response")
	}
	if task := waitTask(t, service); task.Status != "succeeded" {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestUploadFailureKeepsLocalFiles(t *testing.T) {
	repo := mockRepo(t, successScript)
	writeFile(t, filepath.Join(filepath.Dir(repo.binPath), "gate"), "", 0644)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "test-secret")
	}))
	defer server.Close()
	repo.uploadURL, repo.uploadSecret = server.URL, "test-secret"
	service := NewService(repo)
	service.Start()
	task := waitTask(t, service)
	if task.Status != "failed" || !strings.Contains(task.Error, "HTTP 403") || strings.Contains(task.Error, repo.uploadSecret) {
		t.Fatalf("unexpected upload error: %+v", task)
	}
	data, err := os.ReadFile(repo.clashPath)
	if err != nil || len(mappingValue(decodeDocument(t, data), "proxies").Content) != 34 {
		t.Fatal("local Clash missing after upload failure")
	}
	data, err = os.ReadFile(repo.outputPath)
	if err != nil || string(data) != strings.Join(testIPs(), "\n")+"\n" {
		t.Fatal("local IP output missing after upload failure")
	}
}

func TestUploadSkippedAfterSaveFailure(t *testing.T) {
	for _, target := range []string{"IP", "Clash"} {
		t.Run(target, func(t *testing.T) {
			repo := mockRepo(t, successScript)
			writeFile(t, filepath.Join(filepath.Dir(repo.binPath), "gate"), "", 0644)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
			defer server.Close()
			repo.uploadURL, repo.uploadSecret = server.URL, "test-secret"
			if target == "IP" {
				repo.outputPath = t.TempDir()
			} else {
				repo.clashPath = t.TempDir()
			}
			service := NewService(repo)
			service.Start()
			if waitTask(t, service).Status != "failed" || requests.Load() != 0 {
				t.Fatal("upload attempted after save failure")
			}
		})
	}
}

func TestUploadConfigurationAndCancellation(t *testing.T) {
	repo := mockRepo(t, successScript)
	if err := repo.UploadClash(context.Background()); err != nil {
		t.Fatalf("unconfigured upload should skip: %v", err)
	}
	repo.uploadURL = "https://example.com/upload"
	if err := repo.UploadClash(context.Background()); err == nil || !strings.Contains(err.Error(), "upload_config_file_secret") {
		t.Fatal("missing secret not detected")
	}
	repo.uploadURL, repo.uploadSecret = "", "test-secret"
	if err := repo.UploadClash(context.Background()); err == nil || !strings.Contains(err.Error(), "upload_config_file_url") {
		t.Fatal("missing URL not detected")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("canceled upload reached server") }))
	defer server.Close()
	repo.uploadURL = server.URL
	if err := repo.UploadClash(context.Background()); err == nil || !strings.Contains(err.Error(), "读取待上传") {
		t.Fatal("missing saved file not detected")
	}
	if err := repo.SaveClash([]byte("config")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repo.UploadClash(ctx); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancellation ignored: %v", err)
	}
	repo.uploadURL = "://invalid"
	if err := repo.UploadClash(context.Background()); err == nil {
		t.Fatal("invalid upload URL accepted")
	}
}

func TestUploadDoesNotRedirectSecret(t *testing.T) {
	repo := mockRepo(t, successScript)
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	repo.uploadURL, repo.uploadSecret = source.URL, "test-secret"
	if err := repo.SaveClash([]byte("config")); err != nil {
		t.Fatal(err)
	}
	if err := repo.UploadClash(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 307") || forwarded.Load() != 0 {
		t.Fatalf("redirect not rejected: %v", err)
	}
}
