package speedtest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 启动真实 Vite 配置，只将后端目标指向隔离的测试服务，不占用已有开发服务端口。
func TestViteProxy(t *testing.T) {
	if os.Getenv("SPEEDTEST_VITE_INTEGRATION") != "1" {
		t.Skip("set SPEEDTEST_VITE_INTEGRATION=1 with frontend dependencies installed")
	}
	backend, access, _ := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	script := `import {createServer} from 'vite';
 const server=await createServer({server:{host:'127.0.0.1',port:0,open:false,proxy:{'/ocopion/api':{target:process.env.SPEEDTEST_TEST_TARGET}}}});
 await server.listen(); console.log('SPEEDTEST_URL='+server.resolvedUrls.local[0]);`
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "-e", script)
	cmd.Dir = filepath.Join("..", "..", "..", "..", "ocopion-frontend-pc")
	cmd.Env = append(os.Environ(), "SPEEDTEST_TEST_TARGET="+backend.URL)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	urls := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(out)
		for scan.Scan() {
			line := scan.Text()
			if strings.HasPrefix(line, "SPEEDTEST_URL=") {
				urls <- strings.TrimPrefix(line, "SPEEDTEST_URL=")
			}
		}
	}()
	var origin string
	select {
	case origin = <-urls:
	case <-ctx.Done():
		t.Fatal("Vite did not start")
	}
	origin = strings.TrimSuffix(origin, "/")
	req, _ := http.NewRequestWithContext(ctx, "GET", origin+"/ocopion/api/info", nil)
	req.Header.Set("Authorization", "Bearer "+access)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Code int
		Data struct{ NodeName string }
	}
	err = json.NewDecoder(res.Body).Decode(&info)
	res.Body.Close()
	if err != nil || info.Code != 200 || info.Data.NodeName != "Test VPS" {
		t.Fatalf("info: %+v %v", info, err)
	}
	d := websocket.Dialer{Subprotocols: []string{Protocol, "access_token." + access}, HandshakeTimeout: 5 * time.Second}
	conn, _, err := d.DialContext(ctx, "ws"+strings.TrimPrefix(origin, "http")+"/ocopion/api/echo", http.Header{"Origin": []string{"http://frontend.test"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_ = conn.WriteMessage(websocket.TextMessage, []byte("proxy-echo"))
	_, msg, err := conn.ReadMessage()
	if err != nil || string(msg) != "proxy-echo" || conn.Subprotocol() != Protocol {
		t.Fatalf("echo: %s %v", msg, err)
	}
	t.Log(fmt.Sprintf("Vite HTTP rewrite and WebSocket Upgrade/echo verified on %s", origin))
}
