package attach

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type FileRepository struct{}

func NewFileRepository() *FileRepository                     { return &FileRepository{} }
func (*FileRepository) Read(p string) ([]byte, error)        { return os.ReadFile(p) }
func (*FileRepository) Open(p string) (io.ReadCloser, error) { return os.Open(p) }
func (*FileRepository) Size(p string) (int64, error) {
	s, e := os.Stat(p)
	if e != nil {
		return 0, e
	}
	return s.Size(), nil
}
func (*FileRepository) Remove(p string) error {
	e := os.Remove(p)
	if os.IsNotExist(e) {
		return nil
	}
	return e
}
func (*FileRepository) Write(p string, size int64, input io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	n, err := io.Copy(f, io.LimitReader(input, size+1))
	if err == nil && n != size {
		err = fmt.Errorf("文件大小不符合预期（期望 %d 字节）", size)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), p)
}
