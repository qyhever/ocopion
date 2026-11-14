package attach

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"ocopion/internal/config"
	"ocopion/internal/middleware"
	jwtpkg "ocopion/internal/pkg/jwt"
)

func setup(t *testing.T) (*Service, config.AttachConfig) {
	t.Helper()
	d := t.TempDir()
	cfg := config.AttachConfig{UploadDirPath: filepath.Join(d, "uploads"), UploadLargeFilePath: filepath.Join(d, "larges"), ChunkDirPath: filepath.Join(d, "chunks"), ViewAttachBaseURL: "/public/uploads", ViewLargeFileBaseURL: "/public/larges", ChunkDirSalt: "test"}
	return NewService(NewFileRepository(), cfg), cfg
}
func input(data string) VerifyRequest {
	h := md5.Sum([]byte(data))
	return VerifyRequest{hex.EncodeToString(h[:]), "video.MP4", int64(len(data)), 3}
}
func check(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func TestResumeMergeIsolation(t *testing.T) {
	s, cfg := setup(t)
	in := input("abcdefgh")
	v, e := s.Verify(1, in)
	check(t, e)
	if v.UploadedChunks == nil || len(v.UploadedChunks) != 0 {
		t.Fatal(v)
	}
	alt := in
	alt.ChunkSize = 4
	different, err := s.Verify(1, alt)
	check(t, err)
	if different.UploadID == v.UploadID {
		t.Fatal("different chunk layouts share a task")
	}
	check(t, s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 2, 3, strings.NewReader("gh")))
	if _, e = s.Merge(1, MergeRequest{v.UploadID, in.FileMD5, 3}); e == nil {
		t.Fatal("missing chunks accepted")
	}
	// 重启后仍只报告完整落盘的分片。
	s = NewService(NewFileRepository(), cfg)
	v, e = s.Verify(1, in)
	check(t, e)
	if len(v.UploadedChunks) != 1 || v.UploadedChunks[0] != 2 {
		t.Fatal(v)
	}
	other, e := s.Verify(2, in)
	check(t, e)
	if other.UploadID == v.UploadID || len(other.UploadedChunks) != 0 {
		t.Fatal(other)
	}
	if e = s.Chunk(2, v.UploadID, in.FileMD5, in.FileName, 0, 3, strings.NewReader("abc")); e == nil {
		t.Fatal("cross-user access")
	}
	var wg sync.WaitGroup
	for n := 0; n < 5; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 0, 3, strings.NewReader("abc")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	check(t, s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 1, 3, strings.NewReader("def")))
	for n := 0; n < 3; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Merge(1, MergeRequest{v.UploadID, in.FileMD5, 3})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s = NewService(NewFileRepository(), cfg)
	v, e = s.Verify(1, in)
	check(t, e)
	if !v.IsExists {
		t.Fatal(v)
	}
	b, e := os.ReadFile(filepath.Join(cfg.UploadLargeFilePath, filepath.Base(v.URL)))
	check(t, e)
	if string(b) != "abcdefgh" {
		t.Fatal(string(b))
	}
	if _, e = os.Stat(s.chunk(v.UploadID, 0)); !os.IsNotExist(e) {
		t.Fatal("chunks not removed")
	}
	other, e = s.Verify(2, in)
	check(t, e)
	if other.IsExists {
		t.Fatal("cross-user dedup")
	}
}
func TestIntegrityFailureAndChunkLength(t *testing.T) {
	s, _ := setup(t)
	in := input("abcdef")
	v, e := s.Verify(1, in)
	check(t, e)
	for _, bad := range []string{"ab", "abcd"} {
		if e = s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 0, 2, strings.NewReader(bad)); e == nil {
			t.Fatal("wrong size accepted")
		}
	}
	check(t, s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 0, 2, strings.NewReader("xxx")))
	check(t, s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 1, 2, strings.NewReader("def")))
	if _, e = s.Merge(1, MergeRequest{v.UploadID, in.FileMD5, 2}); e == nil {
		t.Fatal("bad md5 accepted")
	}
	v, e = s.Verify(1, in)
	check(t, e)
	if len(v.UploadedChunks) != 0 {
		t.Fatal(v)
	}
	entries, _ := os.ReadDir(s.cfg.UploadLargeFilePath)
	if len(entries) != 0 {
		t.Fatal("bad output published")
	}
	check(t, s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 0, 2, strings.NewReader("abc")))
	check(t, s.Chunk(1, v.UploadID, in.FileMD5, in.FileName, 1, 2, strings.NewReader("def")))
	_, e = s.Merge(1, MergeRequest{v.UploadID, in.FileMD5, 2})
	check(t, e)
}
func TestValidation(t *testing.T) {
	s, _ := setup(t)
	for _, mutate := range []func(*VerifyRequest){
		func(v *VerifyRequest) { v.FileMD5 = "../bad" }, func(v *VerifyRequest) { v.FileName = "../video.mp4" }, func(v *VerifyRequest) { v.FileName = "a.txt" }, func(v *VerifyRequest) { v.FileSize = 0 }, func(v *VerifyRequest) { v.FileSize = MaxFileSize + 1 }, func(v *VerifyRequest) { v.ChunkSize = MaxUploadSize + 1 }, func(v *VerifyRequest) { v.ChunkSize = -1 },
	} {
		v := input("abcdef")
		mutate(&v)
		if _, e := s.Verify(1, v); e == nil {
			t.Fatalf("accepted: %+v", v)
		}
	}
	v := input("abcdef")
	v.ChunkSize = 0
	r, e := s.Verify(1, v)
	check(t, e)
	task, e := s.load(1, r.UploadID, v.FileMD5)
	check(t, e)
	if task.ChunkSize != DefaultChunkSize {
		t.Fatal(task)
	}
	v.FileSize = MaxFileSize
	v.ChunkSize = MaxUploadSize
	_, e = s.Verify(1, v)
	check(t, e)
}

type zeros struct{}

func (zeros) Read(b []byte) (int, error) { clear(b); return len(b), nil }
func TestSimpleUploadBoundaries(t *testing.T) {
	s, _ := setup(t)
	for _, size := range []int64{0, MaxUploadSize} {
		out, e := s.Upload("报告.Any", size, io.LimitReader(zeros{}, size))
		check(t, e)
		if !strings.HasSuffix(out.FileName, ".Any") || out.OriginName != "报告.Any" {
			t.Fatal(out)
		}
	}
	for _, tc := range []struct{ size, actual int64 }{{MaxUploadSize + 1, 0}, {1, 2}, {2, 1}} {
		if _, e := s.Upload("x.txt", tc.size, io.LimitReader(zeros{}, tc.actual)); e == nil {
			t.Fatal(tc)
		}
	}
}
func registerTestRoutes(r *gin.Engine, s *Service) {
	ctl := NewController(s)
	group := r.Group("/api/attach", middleware.JWTAuthMiddleware())
	group.POST("/upload", ctl.Upload)
	group.POST("/verify", ctl.Verify)
	group.POST("/chunk", ctl.Chunk)
	group.POST("/merge", ctl.Merge)
}

func TestHTTPContract(t *testing.T) {
	s, cfg := setup(t)
	old := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = old })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "test"}}
	token, refresh, e := jwtpkg.GenToken(1)
	check(t, e)
	r := gin.New()
	r.Static("/public/uploads", cfg.UploadDirPath)
	registerTestRoutes(r, s)
	for _, path := range []string{"upload", "verify", "chunk", "merge"} {
		for _, auth := range []string{"", refresh, "bad"} {
			req := httptest.NewRequest("POST", "/api/attach/"+path, nil)
			req.Header.Set("Authorization", "Bearer "+auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != 401 {
				t.Fatalf("%s: %d", path, w.Code)
			}
		}
	}
	// 合法 access token 可正常上传。
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, e := mw.CreateFormFile("file", "file.txt")
	check(t, e)
	part.Write([]byte("hello"))
	mw.Close()
	req := httptest.NewRequest("POST", "/api/attach/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp struct {
		Code int
		Data UploadResponse
	}
	check(t, json.Unmarshal(w.Body.Bytes(), &resp))
	if resp.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, resp.Data.URL, nil))
	if w.Code != 200 || w.Body.String() != "hello" {
		t.Fatal(w.Body.String())
	}
	req = httptest.NewRequest("POST", "/api/attach/verify", strings.NewReader("{} {}"))
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "\"code\":-1") {
		t.Fatal(w.Body.String())
	}
}

func TestChunkHTTPFlow(t *testing.T) {
	s, cfg := setup(t)
	old := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = old })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "test"}}
	token, _, e := jwtpkg.GenToken(1)
	check(t, e)
	r := gin.New()
	registerTestRoutes(r, s)
	r.Static("/public/larges", cfg.UploadLargeFilePath)
	send := func(path, content string, body io.Reader) []byte {
		req := httptest.NewRequest("POST", "/api/attach/"+path, body)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", content)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	in := input("abcdef")
	raw, _ := json.Marshal(in)
	b := send("verify", "application/json", bytes.NewReader(raw))
	var vr struct {
		Code int
		Data VerifyResponse
	}
	check(t, json.Unmarshal(b, &vr))
	if vr.Code != 200 {
		t.Fatal(string(b))
	}
	for _, tc := range []struct {
		q, index, data string
		ok             bool
	}{{"0", "1", "abc", false}, {"0", "0", "ab", false}, {"1", "1", "def", true}, {"0", "0", "abc", true}, {"0", "0", "abc", true}} {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range map[string]string{"uploadId": vr.Data.UploadID, "fileMd5": in.FileMD5, "fileName": in.FileName, "chunkIndex": tc.index} {
			check(t, mw.WriteField(k, v))
		}
		f, err := mw.CreateFormFile("chunk", "chunk.part")
		check(t, err)
		f.Write([]byte(tc.data))
		mw.Close()
		b = send("chunk?chunkIndex="+tc.q+"&chunkTotal=2", mw.FormDataContentType(), &buf)
		var resp struct{ Code int }
		check(t, json.Unmarshal(b, &resp))
		if (resp.Code == 200) != tc.ok {
			t.Fatal(string(b))
		}
	}
	raw, _ = json.Marshal(MergeRequest{vr.Data.UploadID, in.FileMD5, 2})
	b = send("merge", "application/json", bytes.NewReader(raw))
	var mr struct {
		Code int
		Data MergeResponse
	}
	check(t, json.Unmarshal(b, &mr))
	if mr.Code != 200 {
		t.Fatal(string(b))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", mr.Data.URL, nil))
	if w.Code != 200 || w.Body.String() != "abcdef" {
		t.Fatal(w.Body.String())
	}
	b = send("verify", "application/json", strings.NewReader(fmtVerify(in)))
	check(t, json.Unmarshal(b, &vr))
	if !vr.Data.IsExists {
		t.Fatal(string(b))
	}
}
func fmtVerify(in VerifyRequest) string { b, _ := json.Marshal(in); return string(b) }
