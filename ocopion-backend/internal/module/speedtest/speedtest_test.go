package speedtest

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"ocopion/internal/config"
	jwtpkg "ocopion/internal/pkg/jwt"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	old := config.GlobalConfig
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "speedtest-test"}}
	t.Cleanup(func() { config.GlobalConfig = old })
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	NewController(NewService(NewRepository(), config.SpeedtestConfig{NodeName: "Test VPS", AllowedOrigins: []string{"http://frontend.test"}})).Register(r.Group("/api"))
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	access, refresh, err := jwtpkg.GenToken(42)
	if err != nil {
		t.Fatal(err)
	}
	return server, access, refresh
}
func TestAuthenticationAndUpload(t *testing.T) {
	s, access, refresh := testServer(t)
	for _, path := range []string{"/info", "/download", "/upload", "/echo"} {
		for _, token := range []string{"", refresh, "invalid"} {
			method := "GET"
			if path == "/upload" {
				method = "POST"
			}
			req, _ := http.NewRequest(method, s.URL+"/api"+path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != 401 {
				t.Fatalf("%s: %d", path, res.StatusCode)
			}
		}
	}
	for _, n := range []int{0, 65536, MaxUpload, MaxUpload + 1} {
		req, _ := http.NewRequest("POST", s.URL+"/api/upload", bytes.NewReader(make([]byte, n)))
		req.Header.Set("Authorization", "Bearer "+access)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Code int
			Data struct{ Bytes int }
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if n <= MaxUpload && (body.Code != 200 || body.Data.Bytes != n) {
			t.Fatalf("confirmation: %+v", body)
		}
		if n > MaxUpload && body.Code == 200 {
			t.Fatal("oversized upload accepted")
		}
	}
	req, _ := http.NewRequest("GET", s.URL+"/api/info", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("X-Forwarded-For", "203.0.113.42")
	res, _ := http.DefaultClient.Do(req)
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if strings.Contains(string(data), "203.0.113.42") || !strings.Contains(string(data), "Test VPS") {
		t.Fatal(string(data))
	}
}
func TestEchoOriginAndProtocol(t *testing.T) {
	s, access, refresh := testServer(t)
	for _, tc := range []struct {
		origin, token string
		ok            bool
	}{{"http://frontend.test", access, true}, {"http://evil.test", access, false}, {"", access, false}, {"http://frontend.test", refresh, false}} {
		d := websocket.Dialer{Subprotocols: []string{Protocol, "access_token." + tc.token}}
		c, res, err := d.Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/api/echo", http.Header{"Origin": []string{tc.origin}})
		if !tc.ok {
			if err == nil {
				c.Close()
				t.Fatal("accepted forbidden socket")
			}
			if res != nil {
				res.Body.Close()
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if c.Subprotocol() != Protocol {
			t.Fatal("credential negotiated")
		}
		_ = c.WriteMessage(websocket.TextMessage, []byte("123"))
		_, msg, err := c.ReadMessage()
		if err != nil || string(msg) != "123" {
			t.Fatalf("%s %v", msg, err)
		}
		c.Close()
	}
}
func TestDownloadCancellation(t *testing.T) {
	s, access, _ := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", s.URL+"/api/download", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"Content-Type": "application/octet-stream", "X-Accel-Buffering": "no", "Content-Encoding": "identity"} {
		if res.Header.Get(key) != value {
			t.Fatal(res.Header)
		}
	}
	if !strings.Contains(res.Header.Get("Cache-Control"), "no-store") {
		t.Fatal(res.Header)
	}
	if _, err = io.ReadFull(res.Body, make([]byte, 1024)); err != nil {
		t.Fatal(err)
	}
	cancel()
	res.Body.Close()
	// ServeHTTP must terminate after cancellation, not wait for the full stream lifetime.
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("download did not terminate")
	}
}

type brokenReader struct{}

func (brokenReader) Read(p []byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestUploadReadFailure(t *testing.T) {
	if _, err := NewService(NewRepository(), config.SpeedtestConfig{}).Upload(brokenReader{}); err == nil {
		t.Fatal("read failure accepted")
	}
}

func TestSpeedtestCannotBeWhitelisted(t *testing.T) {
	s, _, _ := testServer(t)
	config.GlobalConfig.Auth.Whitelist = []string{"/api/info", "/api/upload", "/api/download", "/api/echo"}
	for _, path := range []string{"info", "download", "upload", "echo"} {
		method := "GET"
		if path == "upload" {
			method = "POST"
		}
		req, _ := http.NewRequest(method, s.URL+"/api/"+path, nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Fatalf("whitelist bypassed %s", path)
		}
	}
}

func TestSameOriginAndMessageLimit(t *testing.T) {
	s, access, _ := testServer(t)
	d := websocket.Dialer{Subprotocols: []string{Protocol, "access_token." + access}}
	c, _, err := d.Dial("ws"+strings.TrimPrefix(s.URL, "http")+"/api/echo", http.Header{"Origin": []string{s.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	if err = c.WriteMessage(websocket.BinaryMessage, make([]byte, 1025)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = c.ReadMessage(); err == nil {
		t.Fatal("oversized echo accepted")
	}
}

// 慢速客户端超过原先 5 秒读取限制后，仍能得到完整的服务端确认。
func TestSlowUpload(t *testing.T) {
	s, access, _ := testServer(t)
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		defer writer.Close()
		if _, err := writer.Write([]byte("first")); err != nil {
			return
		}
		time.Sleep(6 * time.Second)
		_, _ = writer.Write([]byte("last"))
	}()
	req, _ := http.NewRequest("POST", s.URL+"/api/upload", reader)
	req.Header.Set("Authorization", "Bearer "+access)
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct {
		Code int
		Data struct{ Bytes int }
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 200 || body.Data.Bytes != 9 {
		t.Fatalf("slow upload confirmation: %+v", body)
	}
}
