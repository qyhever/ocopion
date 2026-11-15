package speedtest

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const MaxUpload = 8 << 20

// Repository 只维护启动时生成的不可变缓冲区，不持久化测试数据。
type Repository struct{ payload []byte }

func NewRepository() *Repository {
	payload := make([]byte, 64<<10)
	if _, err := rand.Read(payload); err != nil {
		panic(err)
	}
	return &Repository{payload: payload}
}
func (r *Repository) Download(ctx context.Context, w http.ResponseWriter) error {
	deadline := time.Now().Add(20 * time.Second)
	rc := http.NewResponseController(w)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := w.Write(r.payload); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) Upload(body io.Reader) (int64, error) { return io.Copy(io.Discard, body) }

func (r *Repository) Echo(conn *websocket.Conn) {
	conn.SetReadLimit(1024)
	end := time.Now().Add(90 * time.Second)
	for time.Now().Before(end) {
		readEnd := time.Now().Add(10 * time.Second)
		if readEnd.After(end) {
			readEnd = end
		}
		_ = conn.SetReadDeadline(readEnd)
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := conn.WriteMessage(kind, data); err != nil {
			return
		}
	}
}
