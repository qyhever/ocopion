package speedtest

import (
	"context"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
	"ocopion/internal/config"
)

type Service struct {
	repo *Repository
	cfg  config.SpeedtestConfig
}

func NewService(repo *Repository, cfg config.SpeedtestConfig) *Service { return &Service{repo, cfg} }
func (s *Service) Info(ip string) map[string]string {
	family := "IPv6"
	if net.ParseIP(ip).To4() != nil {
		family = "IPv4"
	}
	name := s.cfg.NodeName
	if name == "" {
		name = "当前 VPS"
	}
	return map[string]string{"nodeName": name, "clientIP": ip, "addressFamily": family}
}
func (s *Service) Download(ctx context.Context, w http.ResponseWriter) error {
	return s.repo.Download(ctx, w)
}
func (s *Service) Upload(body io.Reader) (int64, error) { return s.repo.Upload(body) }

func (s *Service) Echo(conn *websocket.Conn) { s.repo.Echo(conn) }
