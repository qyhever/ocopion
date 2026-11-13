package attach

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"ocopion/internal/config"
)

var ErrInput = errors.New("附件参数、大小或完整性不符合要求")
var md5Pattern = regexp.MustCompile("^[a-fA-F0-9]{32}$")
var idPattern = regexp.MustCompile("^[a-f0-9]{64}$")
var extPattern = regexp.MustCompile("^\\.[a-zA-Z0-9]{1,16}$")

// Service 面向单实例本地磁盘；固定数量的锁避免任务增多导致锁表无限增长。
type Service struct {
	repo  Repository
	cfg   config.AttachConfig
	locks [256]sync.Mutex
}

func NewService(repo Repository, cfg config.AttachConfig) *Service {
	return &Service{repo: repo, cfg: cfg}
}
func (s *Service) lock(id string) func() {
	i, _ := hex.DecodeString(id[:2])
	m := &s.locks[int(i[0])]
	m.Lock()
	return m.Unlock
}
func validName(n string) bool {
	return n != "" && len(n) <= 255 && n != "." && n != ".." && !strings.ContainsAny(n, "/\\") && !strings.ContainsFunc(n, unicode.IsControl)
}
func randomName(ext string) (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b) + ext, nil
}
func url(base, name string) string { return strings.TrimRight(base, "/") + "/" + name }
func (s *Service) Upload(name string, size int64, input io.Reader) (UploadResponse, error) {
	if !validName(name) || size < 0 || size > MaxUploadSize || s.cfg.UploadDirPath == "" {
		return UploadResponse{}, ErrInput
	}
	ext := filepath.Ext(name)
	if !extPattern.MatchString(ext) {
		ext = ""
	}
	stored, e := randomName(ext)
	if e != nil {
		return UploadResponse{}, e
	}
	if e = s.repo.Write(filepath.Join(s.cfg.UploadDirPath, stored), size, input); e != nil {
		return UploadResponse{}, e
	}
	return UploadResponse{stored, name, url(s.cfg.ViewAttachBaseURL, stored)}, nil
}
func (s *Service) taskID(user uint64, in VerifyRequest) string {
	b, _ := json.Marshal(struct {
		User uint64
		File VerifyRequest
	}{user, in})
	h := hmac.New(sha256.New, []byte(s.cfg.ChunkDirSalt))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Service) meta(id string) string { return filepath.Join(s.cfg.ChunkDirPath, id, "task.json") }
func (s *Service) chunk(id string, index int) string {
	return filepath.Join(s.cfg.ChunkDirPath, id, fmt.Sprintf("%d.part", index))
}
func (s *Service) save(id string, t *Task) error {
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	return s.repo.Write(s.meta(id), int64(len(b)), bytes.NewReader(b))
}
func (s *Service) load(user uint64, id, md5 string) (*Task, error) {
	if !idPattern.MatchString(id) || !md5Pattern.MatchString(md5) {
		return nil, ErrInput
	}
	b, e := s.repo.Read(s.meta(id))
	if e != nil {
		return nil, e
	}
	var t Task
	if json.Unmarshal(b, &t) != nil || t.UserID != user || t.FileMD5 != strings.ToLower(md5) || t.ChunkSize <= 0 || t.FileSize <= 0 || s.taskID(user, t.VerifyRequest) != id {
		return nil, ErrInput
	}
	return &t, nil
}
func (s *Service) Verify(user uint64, in VerifyRequest) (VerifyResponse, error) {
	out := VerifyResponse{UploadedChunks: []int{}}
	if user == 0 || !validName(in.FileName) || !strings.EqualFold(filepath.Ext(in.FileName), ".mp4") || !md5Pattern.MatchString(in.FileMD5) || in.FileSize <= 0 || in.FileSize > MaxFileSize {
		return out, ErrInput
	}
	if in.ChunkSize == 0 {
		in.ChunkSize = DefaultChunkSize
	}
	if in.ChunkSize < 1 || in.ChunkSize > MaxUploadSize || (in.FileSize+in.ChunkSize-1)/in.ChunkSize > 100000 || s.cfg.ChunkDirPath == "" || s.cfg.UploadLargeFilePath == "" {
		return out, ErrInput
	}
	in.FileMD5 = strings.ToLower(in.FileMD5)
	id := s.taskID(user, in)
	unlock := s.lock(id)
	defer unlock()
	t, e := s.load(user, id, in.FileMD5)
	if os.IsNotExist(e) {
		name, err := randomName(".mp4")
		if err != nil {
			return out, err
		}
		t = &Task{VerifyRequest: in, UserID: user, Output: name}
		e = s.save(id, t)
	}
	if e != nil {
		return out, e
	}
	out.UploadID = id
	if t.Complete {
		n, err := s.repo.Size(filepath.Join(s.cfg.UploadLargeFilePath, t.Output))
		if err == nil && n == t.FileSize {
			out.IsExists = true
			out.URL = url(s.cfg.ViewLargeFileBaseURL, t.Output)
			return out, nil
		}
		t.Complete = false
		if e = s.save(id, t); e != nil {
			return out, e
		}
	}
	for i := 0; i < t.total(); i++ {
		if n, err := s.repo.Size(s.chunk(id, i)); err == nil && n == t.expected(i) {
			out.UploadedChunks = append(out.UploadedChunks, i)
		}
	}
	return out, nil
}
func (s *Service) Chunk(user uint64, id, md5, name string, index, total int, input io.Reader) error {
	if !idPattern.MatchString(id) {
		return ErrInput
	}
	unlock := s.lock(id)
	defer unlock()
	t, e := s.load(user, id, md5)
	if e != nil {
		return e
	}
	if name != t.FileName || total != t.total() || index < 0 || index >= total {
		return ErrInput
	}
	if t.Complete {
		n, err := io.Copy(io.Discard, io.LimitReader(input, t.expected(index)+1))
		if err != nil {
			return err
		}
		if n != t.expected(index) {
			return ErrInput
		}
		return nil
	}
	// 重复片也先校验实际读取长度，再原子替换，避免损坏已有完整片。
	return s.repo.Write(s.chunk(id, index), t.expected(index), input)
}
func (s *Service) clearChunks(id string, t *Task) error {
	for i := 0; i < t.total(); i++ {
		if e := s.repo.Remove(s.chunk(id, i)); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) Merge(user uint64, in MergeRequest) (MergeResponse, error) {
	out := MergeResponse{}
	if !idPattern.MatchString(in.UploadID) {
		return out, ErrInput
	}
	unlock := s.lock(in.UploadID)
	defer unlock()
	t, e := s.load(user, in.UploadID, in.FileMD5)
	if e != nil {
		return out, e
	}
	if in.ChunkLength != t.total() {
		return out, ErrInput
	}
	out = MergeResponse{url(s.cfg.ViewLargeFileBaseURL, t.Output), "合并成功"}
	if t.Complete {
		n, err := s.repo.Size(filepath.Join(s.cfg.UploadLargeFilePath, t.Output))
		if err != nil || n != t.FileSize {
			return MergeResponse{}, errors.New("成品文件缺失，请重新预检上传")
		}
		return out, s.clearChunks(in.UploadID, t)
	}
	for i := 0; i < t.total(); i++ {
		n, err := s.repo.Size(s.chunk(in.UploadID, i))
		if err != nil || n != t.expected(i) {
			return MergeResponse{}, errors.New("分片缺失或大小错误，请重新预检上传")
		}
	}
	// 管道按数字索引流式读取，散列失败通过 CloseWithError 阻止 Repository 发布。
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		h := md5.New()
		var total int64
		var err error
		for i := 0; i < t.total(); i++ {
			var f io.ReadCloser
			f, err = s.repo.Open(s.chunk(in.UploadID, i))
			if err != nil {
				break
			}
			var n int64
			n, err = io.Copy(io.MultiWriter(writer, h), f)
			ce := f.Close()
			total += n
			if err == nil {
				err = ce
			}
			if err != nil {
				break
			}
		}
		if err == nil && (total != t.FileSize || hex.EncodeToString(h.Sum(nil)) != t.FileMD5) {
			err = ErrInput
		}
		writer.CloseWithError(err)
		done <- err
	}()
	e = s.repo.Write(filepath.Join(s.cfg.UploadLargeFilePath, t.Output), t.FileSize, reader)
	reader.CloseWithError(e)
	integrity := <-done
	if errors.Is(integrity, ErrInput) {
		if ce := s.clearChunks(in.UploadID, t); ce != nil {
			return MergeResponse{}, ce
		}
	}
	if e != nil {
		return MergeResponse{}, e
	}
	if integrity != nil {
		return MergeResponse{}, integrity
	}
	t.Complete = true
	if e = s.save(in.UploadID, t); e != nil {
		return MergeResponse{}, e
	}
	return out, s.clearChunks(in.UploadID, t)
}
