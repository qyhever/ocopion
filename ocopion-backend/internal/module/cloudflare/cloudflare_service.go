package cloudflare

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Service struct {
	repo    Repository
	mu      sync.Mutex
	task    Task
	timeout time.Duration
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, timeout: 10 * time.Minute, task: Task{Status: "idle", IPs: []string{}}}
}

// SpeedTest 同步测速，只返回结果，不更新后台任务及其输出文件。
func (s *Service) SpeedTest(ctx context.Context) (SpeedTestResult, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	candidates, err := s.repo.MeasureSpeedTest(ctx)
	if err != nil {
		return SpeedTestResult{}, err
	}
	filtered := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Delay <= 200 {
			filtered = append(filtered, candidate)
		}
	}
	ips, err := selectIPCount(filtered, speedTestIPCount)
	if err != nil {
		return SpeedTestResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return SpeedTestResult{}, fmt.Errorf("cfst 测速超时或取消: %w", err)
	}
	return SpeedTestResult{IPs: ips}, nil
}

func (s *Service) Get() Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

func (s *Service) snapshot() Task {
	task := s.task
	task.IPs = append([]string{}, task.IPs...)
	if task.StartedAt != nil {
		t := *task.StartedAt
		task.StartedAt = &t
	}
	if task.FinishedAt != nil {
		t := *task.FinishedAt
		task.FinishedAt = &t
	}
	return task
}

func (s *Service) Start() Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task.Status == "running" {
		return s.snapshot()
	}
	now := time.Now().UTC()
	s.task = Task{Status: "running", StartedAt: &now, IPs: []string{}}
	go s.run()
	return s.snapshot()
}

func (s *Service) run() {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	candidates, err := s.repo.Measure(ctx)
	var ips []string
	var clash []byte
	if err == nil {
		ips, err = selectIPs(candidates)
	}
	if err == nil {
		var template []byte
		template, err = s.repo.ReadClashTemplate()
		if err == nil {
			clash, err = generateClash(template, ips)
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = s.repo.Save(ips)
	}
	if err == nil {
		err = s.repo.SaveClash(clash)
	}
	if err == nil {
		err = s.repo.UploadClash(ctx)
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.task.FinishedAt = &now
	if err != nil {
		s.task.Status = "failed"
		s.task.Error = err.Error()
		return
	}
	s.task.Status = "succeeded"
	s.task.IPs = ips
	zap.L().Info("Cloudflare 优选及配置处理成功",
		zap.String("status", s.task.Status),
		zap.Int("ip_count", len(ips)),
		zap.Strings("ips", ips),
		zap.Time("started_at", *s.task.StartedAt),
		zap.Time("finished_at", now),
		zap.Duration("duration", now.Sub(*s.task.StartedAt)),
	)
}

func selectIPs(candidates []Candidate) ([]string, error) {
	return selectIPCount(candidates, fastIPCount)
}

func selectIPCount(candidates []Candidate, count int) ([]string, error) {
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Delay != b.Delay {
			return a.Delay < b.Delay
		}
		if a.Loss != b.Loss {
			return a.Loss < b.Loss
		}
		return a.IP.Less(b.IP)
	})
	ips := make([]string, 0, count)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		ip := candidate.IP.String()
		if seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
		if len(ips) == count {
			return ips, nil
		}
	}
	return nil, fmt.Errorf("有效 IPv4 不足 %d 个（实际 %d 个）", count, len(ips))
}
