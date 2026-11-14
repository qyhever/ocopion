package txtvideo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "golang.org/x/image/webp"
)

const MaxCoverSize = 10 << 20

var ErrTaskNotFound = errors.New("任务不存在或已过期，请重新生成")

type Task struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	VideoURL    string `json:"videoUrl,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
	owner       uint64
	finished    time.Time
}
type Service struct {
	repo    Repository
	mu      sync.Mutex
	tasks   map[string]Task
	active  map[uint64]bool
	timeout time.Duration
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, tasks: make(map[string]Task), active: make(map[uint64]bool), timeout: 30 * time.Minute}
}
func validate(text, speed string, cover []byte) (string, string, error) {
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 10000 {
		return "", "", fmt.Errorf("请输入 1 至 10000 字的文案")
	}
	if speed == "" {
		speed = "1"
	}
	if speed != "1" && speed != "0.5" {
		return "", "", fmt.Errorf("倍速仅支持 1 或 0.5")
	}
	if len(cover) == 0 || len(cover) > MaxCoverSize {
		return "", "", fmt.Errorf("封面不能为空且不能超过 10 MiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(cover))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") {
		return "", "", fmt.Errorf("封面必须为有效的 JPEG、PNG 或 WebP 图片")
	}
	// Bound decoded memory and reject truncated/invalid images before launching tools.
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
		return "", "", fmt.Errorf("封面尺寸过大，最多支持 4000 万像素")
	}
	if _, _, err := image.Decode(bytes.NewReader(cover)); err != nil {
		return "", "", fmt.Errorf("封面图片损坏或格式不受支持")
	}
	return text, speed, nil
}
func (s *Service) pruneLocked() {
	for id, t := range s.tasks {
		if !t.finished.IsZero() && time.Since(t.finished) >= 24*time.Hour {
			delete(s.tasks, id)
		}
	}
}
func (s *Service) Create(owner uint64, text, speed string, cover []byte) (Task, error) {
	if owner == 0 {
		return Task{}, fmt.Errorf("请先登录")
	}
	text, speed, err := validate(text, speed, cover)
	if err != nil {
		return Task{}, err
	}
	if err := s.repo.CheckDependencies(); err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	s.pruneLocked()
	if s.active[owner] || len(s.active) >= 2 {
		s.mu.Unlock()
		return Task{}, fmt.Errorf("已有任务正在生成或服务繁忙，请稍后重试")
	}
	s.active[owner] = true
	s.mu.Unlock()
	dir, err := s.repo.Prepare(bytes.NewReader(cover))
	if err != nil {
		s.mu.Lock()
		delete(s.active, owner)
		s.mu.Unlock()
		return Task{}, fmt.Errorf("保存封面失败: %w", err)
	}
	task := Task{ID: newID(), Status: "running", owner: owner}
	s.mu.Lock()
	s.tasks[task.ID] = task
	s.mu.Unlock()
	go s.run(task, dir, text, speed)
	return task, nil
}
func (s *Service) run(task Task, dir, text, speed string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	err := s.repo.Generate(ctx, dir, text, speed, task.ID+".mp4")
	s.repo.Cleanup(dir)
	task.finished = time.Now()
	if err != nil {
		task.Status, task.Error = "failed", err.Error()
		if errors.Is(err, context.DeadlineExceeded) {
			task.Error = "生成超过 30 分钟，请缩短文案后重试"
		}
	} else {
		task.Status = "succeeded"
		task.VideoURL = "/ocopion/api/txt-video/files/" + task.ID + ".mp4"
		task.DownloadURL = task.VideoURL + "?download=1"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[task.ID] = task
	delete(s.active, task.owner)
	// Expire idle services too; no permanent janitor goroutine is required.
	time.AfterFunc(24*time.Hour, func() { s.mu.Lock(); defer s.mu.Unlock(); s.pruneLocked() })
}
func (s *Service) Get(owner uint64, id string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	task, ok := s.tasks[id]
	if !ok || owner == 0 || task.owner != owner {
		return Task{}, ErrTaskNotFound
	}
	return task, nil
}
func (s *Service) Open(filename string) (*os.File, error) { return s.repo.Open(filename) }

// ReadCover bounds allocation even when a multipart header lies about its size.
func ReadCover(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxCoverSize+1))
	if err == nil && len(b) > MaxCoverSize {
		err = fmt.Errorf("封面不能超过 10 MiB")
	}
	return b, err
}
