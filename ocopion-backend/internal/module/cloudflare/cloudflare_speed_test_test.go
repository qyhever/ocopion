package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestSpeedTestHTTP(t *testing.T) {
	repo := mockRepo(t, successScript)
	dir := filepath.Dir(repo.binPath)
	writeFile(t, filepath.Join(dir, "gate"), "", 0644)
	// 验证同步接口不依赖模板，也不改写既有输出。
	os.Remove(repo.templatePath)
	for _, path := range []string{repo.outputPath, repo.clashPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, "old", 0644)
	}
	s := NewService(repo)
	before := s.Get()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/cloudflare/speed-test", NewController(s).SpeedTest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/api/cloudflare/speed-test", nil))
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    SpeedTestResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 200 || envelope.Message != "操作成功" || len(envelope.Data.IPs) != 12 || envelope.Data.IPs[11] != "1.1.1.12" {
		t.Fatalf("bad response: %s", response.Body)
	}
	if !reflect.DeepEqual(before, s.Get()) {
		t.Fatal("background state changed")
	}
	for _, path := range []string{repo.outputPath, repo.clashPath} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "old" {
			t.Fatalf("output changed: %s, %v", data, err)
		}
	}
	cwd, _ := os.ReadFile(filepath.Join(dir, "cwd.txt"))
	actual, _ := filepath.EvalSymlinks(strings.TrimSpace(string(cwd)))
	expected, _ := filepath.EvalSymlinks(dir)
	if actual != expected {
		t.Fatalf("wrong cwd: %s", cwd)
	}
	args := speedTestArgs(t, dir)
	if len(args) != 11 || !reflect.DeepEqual(args[:8], []string{"-tl", "200", "-p", "12", "-dd", "-f", filepath.Join(dir, "ip.txt"), "-o"}) || args[9] != "-url" || args[10] != repo.url {
		t.Fatalf("bad args: %q", args)
	}
	assertSpeedTestCleaned(t, args)
}

func speedTestArgs(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func assertSpeedTestCleaned(t *testing.T, args []string) {
	t.Helper()
	for i, arg := range args {
		if arg == "-o" {
			if _, err := os.Stat(filepath.Dir(args[i+1])); !os.IsNotExist(err) {
				t.Fatalf("CSV directory leaked: %v", err)
			}
			return
		}
	}
	t.Fatal("missing output argument")
}

func TestSpeedTestCSV(t *testing.T) {
	base := csvHeader
	for i := 1; i <= 11; i++ {
		base += fmt.Sprintf("1.1.1.%d,4,4,0,%d,0\n", i, i)
	}
	for _, tt := range []struct{ name, csv, want string }{
		{"boundary", base + "1.1.1.12,4,4,0,200,0\n1.1.1.13,4,4,0,200.001,0\n1.1.1.1,4,4,0,0,0\nbad,4,4,0,1,0\n::1,4,4,0,1,0\n", ""},
		{"insufficient", base + "1.1.1.12,4,4,0,200.001,0\n1.1.1.1,4,4,0,0,0\nbad,4,4,0,1,0\n", "实际 11 个"},
		{"empty", csvHeader, "实际 0 个"},
		{"invalid rows", csvHeader + "bad,4,4,0,1,0\n1.1.1.1,4,4,0,NaN,0\n", "实际 0 个"},
		{"empty file", "", "CSV"},
		{"bad header", "bad,header\n", "CSV"},
		{"bad row", base + "1.1.1.12,4\n", "CSV"},
		{"bad quote", base + "\"unterminated", "CSV"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := mockRepo(t, successScript)
			dir := filepath.Dir(repo.binPath)
			writeFile(t, filepath.Join(dir, "gate"), "", 0644)
			writeFile(t, filepath.Join(dir, "fixture.csv"), tt.csv, 0644)
			s := NewService(repo)
			result, err := s.SpeedTest(context.Background())
			if tt.want == "" {
				if err != nil || len(result.IPs) != 12 || result.IPs[11] != "1.1.1.12" {
					t.Fatalf("bad boundary result: %+v %v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %s, got %v", tt.want, err)
			}
			assertSpeedTestCleaned(t, speedTestArgs(t, dir))
		})
	}
}

func TestSpeedTestFailures(t *testing.T) {
	for _, tt := range []struct {
		name, script, want string
		timeout            time.Duration
		cancel             bool
	}{
		{"exit", "printf diagnostic; exit 7", "diagnostic", time.Second, false},
		{"timeout", "exec sleep 30", "context deadline exceeded", time.Second, false},
		{"cancel", "exec sleep 30", "context canceled", time.Minute, true},
		{"missing CSV", "exit 0", "CSV", time.Second, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := mockRepo(t, `printf '%s\n' "$@" > args.txt
`+tt.script)
			s := NewService(repo)
			s.timeout = tt.timeout
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := s.SpeedTest(ctx); done <- err }()
			if tt.cancel {
				deadline := time.Now().Add(time.Second)
				for {
					if _, err := os.Stat(filepath.Join(filepath.Dir(repo.binPath), "args.txt")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("program did not start")
					}
					time.Sleep(time.Millisecond)
				}
				cancel()
			}
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("want %s got %v", tt.want, err)
				}
				if tt.cancel && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel cause missing: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("program not terminated")
			}
			assertSpeedTestCleaned(t, speedTestArgs(t, filepath.Dir(repo.binPath)))
			if s.Get().Status != "idle" {
				t.Fatal("background status changed")
			}
		})
	}
}

func TestSpeedTestHTTPFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", NewController(NewService(NewRepository(nil))).SpeedTest)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/", nil))
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != -1 || !strings.Contains(envelope.Message, "cfst_bin_path") {
		t.Fatalf("bad error: %s", response.Body)
	}
}
