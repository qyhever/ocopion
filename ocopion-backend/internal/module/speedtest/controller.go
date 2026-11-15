package speedtest

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"ocopion/internal/middleware"
	"ocopion/internal/pkg/response"
)

const Protocol = "ocopion-speedtest"

type Controller struct{ service *Service }

func NewController(service *Service) *Controller { return &Controller{service} }
func (ctl *Controller) Register(api *gin.RouterGroup) {
	auth := middleware.RequiredJWTAuthMiddleware()
	api.GET("/info", auth, ctl.Info)
	api.GET("/download", auth, ctl.Download)
	api.POST("/upload", auth, ctl.Upload)
	api.GET("/echo", socketAuth, auth, ctl.Echo)
}

// 仅此路由接受子协议凭据；禁止 URL token 和 cookie 回退。
func socketAuth(c *gin.Context) {
	token := ""
	protocol := false
	for _, p := range websocket.Subprotocols(c.Request) {
		if p == Protocol {
			protocol = true
		}
		if strings.HasPrefix(p, "access_token.") {
			token = strings.TrimPrefix(p, "access_token.")
		}
	}
	if !protocol || token == "" {
		response.Unauthorized(c, "缺少测速连接凭据")
		return
	}
	c.Request.Header.Set("Authorization", "Bearer "+token)
	c.Next()
}
func (ctl *Controller) Info(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	response.Success(c, ctl.service.Info(c.ClientIP()))
}
func (ctl *Controller) Download(c *gin.Context) {
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Cache-Control", "no-store, no-transform")
	c.Header("Content-Encoding", "identity")
	c.Header("X-Accel-Buffering", "no")
	_ = ctl.service.Download(c.Request.Context(), c.Writer)
}
func (ctl *Controller) Upload(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(10 * time.Second))
	defer http.NewResponseController(c.Writer).SetReadDeadline(time.Time{})
	body := http.MaxBytesReader(c.Writer, c.Request.Body, MaxUpload)
	defer body.Close()
	n, err := ctl.service.Upload(body)
	if err != nil {
		response.Fail(c, "上传未完整接收或超过 8 MiB")
		return
	}
	response.Success(c, gin.H{"bytes": n})
}
func (ctl *Controller) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return false
	}
	for _, allowed := range ctl.service.cfg.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}
func (ctl *Controller) Echo(c *gin.Context) {
	upgrader := websocket.Upgrader{Subprotocols: []string{Protocol}, CheckOrigin: ctl.checkOrigin, HandshakeTimeout: 5 * time.Second, EnableCompression: false}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ctl.service.Echo(conn)
}
