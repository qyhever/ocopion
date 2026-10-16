package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"ocopion/internal/config"
)

const csvHeader = "IP 地址,已发送,已接收,丢包率,平均延迟,下载速度(MB/s)\n"

func fixtureCSV() string {
	s := csvHeader
	for i := 20; i >= 1; i-- {
		s += fmt.Sprintf("1.1.1.%d,4,4,0,%d,0\n", i, i)
	}
	return s
}
func TestCSVSelection(t *testing.T) {
	data := csvHeader + "1.1.1.10,4,4,0,1,0\n1.1.1.2,4,4,0,1,0\n1.1.1.1,4,3,0.25,1,0\n1.1.1.2,4,4,0,20,0\n"
	for _, row := range []string{"::1,4,4,0,0,0", "bad,4,4,0,0,0", "1.1.1.99,4,0,1,0,0", "1.1.1.98,4,5,0,0,0", "1.1.1.97,4,4,0,NaN,0", "1.1.1.96,4,4,0,+Inf,0", "1.1.1.95,4,4,0,-1,0", "1.1.1.94,4,4,NaN,1,0", "1.1.1.93,0,0,0,1,0", "1.1.1.92,4,4,2,1,0", "1.1.1.91,no,4,0,1,0"} {
		data += row + "\n"
	}
	for i := 3; i <= 20; i++ {
		if i != 10 {
			data += fmt.Sprintf("1.1.1.%d,4,4,0,%d,0\n", i, i)
		}
	}
	candidates, err := parseCSV(strings.NewReader("\ufeff" + data))
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 21 {
		t.Fatalf("invalid rows accepted: %d", len(candidates))
	}
	ips, err := selectIPs(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 16 || !reflect.DeepEqual(ips[:3], []string{"1.1.1.2", "1.1.1.10", "1.1.1.1"}) {
		t.Fatalf("unexpected order: %v", ips)
	}
	if _, err = selectIPs(candidates[:3]); err == nil {
		t.Fatal("expected insufficient results")
	}
	for _, bad := range []string{"", "wrong,header\n", csvHeader + "1.1.1.1,4\n", csvHeader + "\"unterminated"} {
		if _, err := parseCSV(strings.NewReader(bad)); err == nil {
			t.Fatalf("accepted bad CSV %q", bad)
		}
	}
}
func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}
func mockRepo(t *testing.T, script string) *FileRepository {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ip.txt"), "1.1.1.0/24\n", 0644)
	writeFile(t, filepath.Join(dir, "fixture.csv"), fixtureCSV(), 0644)
	bin := filepath.Join(dir, "cfst mock")
	writeFile(t, bin, "#!/bin/sh\n"+script, 0755)
	return NewRepository(&config.Config{CFSTBinPath: bin, OutputFastIPPath: filepath.Join(dir, "output", "fast-ip.txt"), CFSTURL: "https://example.com/test?x=1&y=2"})
}

const successScript = `pwd > cwd.txt
printf '%s\n' "$@" > args.txt
while [ ! -f gate ]; do sleep 0.01; done
while [ "$#" -gt 0 ]; do
 if [ "$1" = '-o' ]; then shift; cp fixture.csv "$1"; fi
 shift
done
`

func waitTask(t *testing.T, s *Service) Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task := s.Get()
		if task.Status != "running" {
			return task
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("task did not finish")
	return Task{}
}
func TestBackgroundTaskAndHTTP(t *testing.T) {
	repo := mockRepo(t, successScript)
	s := NewService(repo)
	if s.Get().Status != "idle" {
		t.Fatal("expected idle")
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	c := NewController(s)
	r.POST("/api/cloudflare/fast-ip", c.Start)
	r.GET("/api/cloudflare/fast-ip", c.Get)
	ctx, cancel := context.WithCancel(context.Background())
	result := httptest.NewRecorder()
	r.ServeHTTP(result, httptest.NewRequest("POST", "/api/cloudflare/fast-ip", nil).WithContext(ctx))
	cancel()
	var response struct {
		Code int  `json:"code"`
		Data Task `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	first := response.Data
	if response.Code != 200 || first.Status != "running" || first.StartedAt == nil {
		t.Fatalf("bad response: %s", result.Body)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task := s.Start()
			if !task.StartedAt.Equal(*first.StartedAt) {
				t.Error("duplicate task")
			}
			_ = s.Get()
		}()
	}
	wg.Wait()
	*first.StartedAt = time.Time{}
	if s.Get().StartedAt.IsZero() {
		t.Fatal("mutable time snapshot")
	}
	dir := filepath.Dir(repo.binPath)
	writeFile(t, filepath.Join(dir, "gate"), "", 0644)
	task := waitTask(t, s)
	if task.Status != "succeeded" || len(task.IPs) != 16 || task.FinishedAt == nil {
		t.Fatalf("unexpected task: %+v", task)
	}
	content, err := os.ReadFile(repo.outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != strings.Join(task.IPs, "\n")+"\n" || strings.Count(string(content), "\n") != 16 {
		t.Fatalf("bad file: %s", content)
	}
	cwd, _ := os.ReadFile(filepath.Join(dir, "cwd.txt"))
	actual, _ := filepath.EvalSymlinks(strings.TrimSpace(string(cwd)))
	expected, _ := filepath.EvalSymlinks(dir)
	if actual != expected {
		t.Fatalf("wrong cwd %q", cwd)
	}
	argsFile, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	args := strings.Split(strings.TrimSpace(string(argsFile)), "\n")
	if len(args) != 9 || !reflect.DeepEqual(args[:5], []string{"-dd", "-p", "0", "-f", filepath.Join(dir, "ip.txt")}) || args[5] != "-o" || args[7] != "-url" || args[8] != repo.url {
		t.Fatalf("wrong args: %q", args)
	}
	if _, err := os.Stat(filepath.Dir(args[6])); !os.IsNotExist(err) {
		t.Fatal("CSV temp directory leaked")
	}
	task.IPs[0] = "changed"
	if s.Get().IPs[0] == "changed" {
		t.Fatal("mutable IP snapshot")
	}
	if s.Start().Status != "running" {
		t.Fatal("cannot restart")
	}
	waitTask(t, s)
}
func TestFailuresPreserveFile(t *testing.T) {
	tests := []struct {
		name, script, want string
		timeout            time.Duration
	}{
		{"exit", "printf 'diagnostic'; exit 7", "diagnostic", 5 * time.Second},
		{"timeout", "exec sleep 30", "context deadline exceeded", 20 * time.Millisecond},
		{"missing CSV", "exit 0", "CSV", 5 * time.Second},
		{"bad CSV", `while [ "$#" -gt 0 ]; do if [ "$1" = '-o' ]; then shift; printf 'bad' > "$1"; fi; shift; done`, "CSV", 5 * time.Second},
		{"insufficient", `while [ "$#" -gt 0 ]; do if [ "$1" = '-o' ]; then shift; head -n 3 fixture.csv > "$1"; fi; shift; done`, "不足", 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := mockRepo(t, tt.script)
			if err := os.MkdirAll(filepath.Dir(repo.outputPath), 0755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, repo.outputPath, "old\n", 0644)
			s := NewService(repo)
			s.timeout = tt.timeout
			s.Start()
			task := waitTask(t, s)
			if task.Status != "failed" || !strings.Contains(task.Error, tt.want) || task.FinishedAt == nil || len(task.IPs) != 0 {
				t.Fatalf("bad task: %+v", task)
			}
			data, _ := os.ReadFile(repo.outputPath)
			if string(data) != "old\n" {
				t.Fatal("old file changed")
			}
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.GET("/", NewController(s).Get)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
			var envelope struct {
				Code int  `json:"code"`
				Data Task `json:"data"`
			}
			json.Unmarshal(response.Body.Bytes(), &envelope)
			if envelope.Code != 200 || envelope.Data.Status != "failed" {
				t.Fatalf("bad GET response %s", response.Body)
			}
		})
	}
}
func TestConfigurationAndSaveFailures(t *testing.T) {
	for _, cfg := range []*config.Config{nil, {CFSTBinPath: "missing-cfst"}} {
		s := NewService(NewRepository(cfg))
		s.Start()
		if waitTask(t, s).Status != "failed" {
			t.Fatal("expected config failure")
		}
	}
	repo := mockRepo(t, successScript)
	os.Remove(filepath.Join(filepath.Dir(repo.binPath), "ip.txt"))
	if _, err := repo.Measure(context.Background()); err == nil {
		t.Fatal("accepted missing ip.txt")
	}
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFile(t, blocker, "old", 0644)
	repo.outputPath = filepath.Join(blocker, "fast-ip.txt")
	candidates, _ := parseCSV(strings.NewReader(fixtureCSV()))
	ips, _ := selectIPs(candidates)
	if err := repo.Save(ips); err == nil {
		t.Fatal("expected directory failure")
	}
	repo.outputPath = t.TempDir()
	if err := repo.Save(ips); err == nil {
		t.Fatal("expected rename failure")
	}
	entries, _ := os.ReadDir(repo.outputPath)
	if len(entries) != 0 {
		t.Fatal("temp output leaked")
	}
	b := &limitedOutput{}
	if n, _ := b.Write([]byte(strings.Repeat("x", outputLimit*3))); n != outputLimit*3 || len(b.data) != outputLimit {
		t.Fatal("unbounded output")
	}
}

func TestRelativePathsAndOptionalURL(t *testing.T) {
	repo := mockRepo(t, successScript)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relativeBin, err := filepath.Rel(cwd, repo.binPath)
	if err != nil {
		t.Fatal(err)
	}
	relativeOutput, err := filepath.Rel(cwd, repo.outputPath)
	if err != nil {
		t.Fatal(err)
	}
	resolved := NewRepository(&config.Config{CFSTBinPath: relativeBin, OutputFastIPPath: relativeOutput})
	if resolved.binPath != repo.binPath || resolved.outputPath != repo.outputPath {
		t.Fatal("paths not resolved against process cwd")
	}
	dir := filepath.Dir(repo.binPath)
	writeFile(t, filepath.Join(dir, "gate"), "", 0644)
	if _, err := resolved.Measure(context.Background()); err != nil {
		t.Fatal(err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	if strings.Contains(string(args), "-url") {
		t.Fatalf("unexpected optional URL: %s", args)
	}
	if err := os.Remove(repo.binPath); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.Measure(context.Background()); err == nil || !strings.Contains(err.Error(), "cfst 执行失败") {
		t.Fatalf("missing executable error: %v", err)
	}
}

func TestSaveFailureTask(t *testing.T) {
	repo := mockRepo(t, successScript)
	dir := filepath.Dir(repo.binPath)
	writeFile(t, filepath.Join(dir, "gate"), "", 0644)
	// 目标是非空目录时，重命名失败，既有内容必须保留。
	if err := os.MkdirAll(repo.outputPath, 0755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(repo.outputPath, "old.txt")
	writeFile(t, old, "old\n", 0644)
	s := NewService(repo)
	s.Start()
	task := waitTask(t, s)
	if task.Status != "failed" || !strings.Contains(task.Error, "替换结果文件失败") {
		t.Fatalf("unexpected task %+v", task)
	}
	content, _ := os.ReadFile(old)
	if string(content) != "old\n" {
		t.Fatal("old output changed")
	}
	entries, _ := os.ReadDir(filepath.Dir(repo.outputPath))
	if len(entries) != 1 {
		t.Fatalf("temporary file leaked: %v", entries)
	}
}
