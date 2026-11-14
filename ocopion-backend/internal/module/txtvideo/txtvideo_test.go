package txtvideo

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"ocopion/internal/config"
)

func pngCover(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestValidation(t *testing.T) {
	cover := pngCover(t)
	for _, tc := range []struct {
		name, text, speed string
		data              []byte
		valid             bool
	}{
		{"default", "  你好  ", "", cover, true}, {"half", "你好", "0.5", cover, true}, {"max", strings.Repeat("中", 10000), "1", cover, true},
		{"empty", " \n", "1", cover, false}, {"too-long", strings.Repeat("中", 10001), "1", cover, false}, {"speed", "你好", "2", cover, false},
		{"fake", "你好", "1", []byte("fake.png"), false}, {"truncated", "你好", "1", cover[:len(cover)/2], false}, {"oversize", "你好", "1", make([]byte, MaxCoverSize+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := validate(tc.text, tc.speed, tc.data)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
	if _, err := ReadCover(bytes.NewReader(make([]byte, MaxCoverSize+1))); err == nil {
		t.Fatal("oversize accepted")
	}
}

type fakeRunner struct {
	calls         [][]string
	missing       string
	failCall      int
	block         bool
	beforePublish func()
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	if name == f.missing {
		return "", errors.New("missing")
	}
	return name, nil
}
func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	f.calls = append(f.calls, append([]string{name}, args...))
	if len(f.calls) == f.failCall {
		return []byte("tool failed"), errors.New("failed")
	}
	if name == "ffprobe" {
		return []byte("2.0\n"), nil
	}
	if f.beforePublish != nil && strings.HasSuffix(args[len(args)-1], "video.mp4") {
		f.beforePublish()
	}
	return nil, os.WriteFile(args[len(args)-1], []byte("generated"), 0600)
}
func testRepo(t *testing.T, runner CommandRunner) *FileRepository {
	t.Helper()
	root := t.TempDir()
	r := NewFileRepository(config.AttachConfig{UploadDirPath: filepath.Join(root, "uploads"), TxtVideoDirPath: filepath.Join(root, "videos")})
	r.runner = runner
	return r
}
func TestRepositoryFlow(t *testing.T) {
	for _, speed := range []string{"1", "0.5"} {
		t.Run(speed, func(t *testing.T) {
			runner := &fakeRunner{}
			r := testRepo(t, runner)
			dir, err := r.Prepare(bytes.NewReader(pngCover(t)))
			if err != nil {
				t.Fatal(err)
			}
			name := newID() + ".mp4"
			runner.beforePublish = func() {
				if _, err := os.Stat(filepath.Join(r.videoDir, name)); !os.IsNotExist(err) {
					t.Fatal("published before completion")
				}
			}
			text := "你好； $(touch /tmp/unsafe)"
			if err := r.Generate(context.Background(), dir, text, speed, name); err != nil {
				t.Fatal(err)
			}
			if len(runner.calls) != 5 {
				t.Fatalf("calls: %v", runner.calls)
			}
			for _, want := range []string{"atempo=" + speed, "-b:a 192k", "-ar 44100", "-ac 2"} {
				if !strings.Contains(strings.Join(runner.calls[1], " "), want) {
					t.Fatal(want)
				}
			}
			for _, want := range []string{"-t 2.000000", "libx264", "bt709", "+faststart", "1920:1080"} {
				if !strings.Contains(strings.Join(runner.calls[4], " "), want) {
					t.Fatal(want)
				}
			}
			data, _ := os.ReadFile(filepath.Join(dir, "text.txt"))
			if string(data) != text {
				t.Fatal("text changed")
			}
			r.Cleanup(dir)
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("cover retained")
			}
			files, _ := os.ReadDir(r.videoDir)
			if len(files) != 1 || files[0].Name() != name {
				t.Fatalf("staging remains: %v", files)
			}
			f, err := r.Open(name)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			for _, bad := range []string{"../" + name, "foo.mp4", name + ".mp4"} {
				if _, err := r.Open(bad); err == nil {
					t.Fatal("bad filename accepted")
				}
			}
		})
	}
}
func TestRepositoryErrors(t *testing.T) {
	r := testRepo(t, &fakeRunner{missing: "edge-tts"})
	if err := r.CheckDependencies(); err == nil || !strings.Contains(err.Error(), "edge-tts") {
		t.Fatal(err)
	}
	for _, fail := range []int{1, 2, 3, 4, 5} {
		t.Run(string(rune('0'+fail)), func(t *testing.T) {
			r := testRepo(t, &fakeRunner{failCall: fail})
			dir, err := r.Prepare(bytes.NewReader(pngCover(t)))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Cleanup(dir)
			if err := r.Generate(context.Background(), dir, "text", "1", newID()+".mp4"); err == nil {
				t.Fatal("failure ignored")
			}
			entries, _ := os.ReadDir(r.videoDir)
			if len(entries) != 0 {
				t.Fatal("partial retained")
			}
		})
	}
}
func awaitTask(t *testing.T, s *Service, owner uint64, id string) Task {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, err := s.Get(owner, id)
		if err != nil {
			t.Fatal(err)
		}
		if v.Status != "running" {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("task did not finish")
	return Task{}
}
func TestTaskLimitsIsolationTimeout(t *testing.T) {
	r := testRepo(t, &fakeRunner{block: true})
	s := NewService(r)
	s.timeout = 50 * time.Millisecond
	cover := pngCover(t)
	a, err := s.Create(1, "text", "", cover)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(2, a.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatal("owner isolation failed")
	}
	if _, err = s.Create(1, "text", "", cover); err == nil {
		t.Fatal("same user twice")
	}
	b, err := s.Create(2, "text", "", cover)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(3, "text", "", cover); err == nil {
		t.Fatal("global limit ignored")
	}
	for owner, id := range map[uint64]string{1: a.ID, 2: b.ID} {
		if v := awaitTask(t, s, owner, id); v.Status != "failed" || !strings.Contains(v.Error, "30 分钟") || v.VideoURL != "" {
			t.Fatal(v)
		}
	}
	dirs, _ := os.ReadDir(r.uploadDir)
	if len(dirs) != 0 {
		t.Fatal("temporary cover retained")
	}
	s.mu.Lock()
	n := len(s.active)
	v := s.tasks[a.ID]
	v.finished = time.Now().Add(-25 * time.Hour)
	s.tasks[a.ID] = v
	s.mu.Unlock()
	if n != 0 {
		t.Fatal("slots retained")
	}
	if _, err := s.Get(1, a.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatal("expired retained")
	}
}
func TestTaskSuccessAndFailure(t *testing.T) {
	for _, fail := range []int{0, 5} {
		t.Run(string(rune('0'+fail)), func(t *testing.T) {
			r := testRepo(t, &fakeRunner{failCall: fail})
			s := NewService(r)
			task, err := s.Create(1, "text", "1", pngCover(t))
			if err != nil {
				t.Fatal(err)
			}
			task = awaitTask(t, s, 1, task.ID)
			if fail == 0 {
				if task.Status != "succeeded" || task.DownloadURL == "" {
					t.Fatal(task)
				}
			} else if task.Status != "failed" || task.VideoURL != "" {
				t.Fatal(task)
			}
			dirs, _ := os.ReadDir(r.uploadDir)
			if len(dirs) != 0 {
				t.Fatal("cover retained")
			}
		})
	}
}
func TestFileRangeAndDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := testRepo(t, &fakeRunner{})
	os.MkdirAll(r.videoDir, 0755)
	name := newID() + ".mp4"
	os.WriteFile(filepath.Join(r.videoDir, name), []byte("0123456789"), 0600)
	ctl := NewController(NewService(r))
	router := gin.New()
	router.GET("/files/:filename", ctl.File)
	req := httptest.NewRequest("GET", "/files/"+name+"?download=1", nil)
	req.Header.Set("Range", "bytes=2-5")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 206 || w.Body.String() != "2345" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("%d %s %v", w.Code, w.Body.String(), w.Header())
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/files/foo.mp4", nil))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}

func TestCommandOutputBounded(t *testing.T) {
	var out limitedBuffer
	first := bytes.Repeat([]byte("x"), 20000)
	if n, err := out.Write(first); n != len(first) || err != nil {
		t.Fatal(n, err)
	}
	out.Write([]byte("failure reason"))
	if len(out.data) != 8192 || !bytes.HasSuffix(out.data, []byte("failure reason")) {
		t.Fatal("output not bounded or lost final error")
	}
}
