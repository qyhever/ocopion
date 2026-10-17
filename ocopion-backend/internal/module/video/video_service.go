package video

import (
	"strings"

	"ocopion/internal/config"
)

type VideoService struct {
	repo Repository
	cfg  *config.Config
}

func NewVideoService(repo Repository, cfg *config.Config) *VideoService {
	return &VideoService{repo: repo, cfg: cfg}
}

func (s *VideoService) GetVideoList() ([]VideoConfig, error) {
	list, err := s.repo.GetVideoList()
	if err != nil {
		return nil, err
	}

	if s.cfg == nil || strings.TrimSpace(s.cfg.File.UploadViewBaseURL) == "" {
		return list, nil
	}

	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.File.UploadViewBaseURL), "/")
	list = append([]VideoConfig(nil), list...)
	for i := range list {
		list[i].FileName = withPublicBaseURL(baseURL, list[i].FileName)
		list[i].Cover = withPublicBaseURL(baseURL, list[i].Cover)
	}

	return list, nil
}

func withPublicBaseURL(baseURL, name string) string {
	if name == "" || strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		return name
	}
	return baseURL + "/" + strings.TrimLeft(name, "/")
}
