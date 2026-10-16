package cloudflare

import (
	"net/netip"
	"time"
)

const fastIPCount = 16

// Task 是当前实例最近一次测速任务的快照。
type Task struct {
	Status     string     `json:"status" enums:"idle,running,succeeded,failed"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	IPs        []string   `json:"ips"`
	Error      string     `json:"error"`
}

type Candidate struct {
	IP    netip.Addr
	Delay float64
	Loss  float64
}
