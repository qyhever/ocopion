package attach

import "io"

// Repository 原子写入，失败不发布不完整文件。
type Repository interface {
	Read(path string) ([]byte, error)
	Open(path string) (io.ReadCloser, error)
	Size(path string) (int64, error)
	Write(path string, size int64, input io.Reader) error
	Remove(path string) error
}
