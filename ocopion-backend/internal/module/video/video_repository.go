package video

type Repository interface {
	GetVideoList() ([]VideoConfig, error)
}
