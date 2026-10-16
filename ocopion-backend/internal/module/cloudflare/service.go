package cloudflare

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
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
	if err == nil {
		ips, err = selectIPs(candidates)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = s.repo.Save(ips)
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
}

func selectIPs(candidates []Candidate) ([]string, error) {
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
	ips := make([]string, 0, fastIPCount)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		ip := candidate.IP.String()
		if seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
		if len(ips) == fastIPCount {
			return ips, nil
		}
	}
	return nil, fmt.Errorf("有效 IPv4 不足 %d 个（实际 %d 个）", fastIPCount, len(ips))
}
