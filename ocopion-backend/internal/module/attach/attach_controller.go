package attach

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"ocopion/internal/middleware"
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type Controller struct{ service *Service }

func NewController(s *Service) *Controller { return &Controller{s} }

func user(c *gin.Context) uint64 { return c.MustGet(middleware.ContextUserIDKey).(uint64) }
func result(c *gin.Context, data interface{}, err error) {
	if err != nil {
		response.Fail(c, err.Error())
		return
	}
	response.Success(c, data)
}
func decode(c *gin.Context, v interface{}) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	d := json.NewDecoder(c.Request.Body)
	if e := d.Decode(v); e != nil {
		return ErrInput
	}
	if d.Decode(new(interface{})) != io.EOF {
		return ErrInput
	}
	return nil
}
func parseMultipart(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxUploadSize+(1<<20))
	return c.Request.ParseMultipartForm(1 << 20)
}
func cleanup(c *gin.Context) {
	if c.Request.MultipartForm != nil {
		c.Request.MultipartForm.RemoveAll()
	}
}

// Upload POST /api/attach/upload，任意格式，最大 100MiB。
func (ctl *Controller) Upload(c *gin.Context) {
	defer cleanup(c)
	if e := parseMultipart(c); e != nil {
		result(c, nil, ErrInput)
		return
	}
	f, h, e := c.Request.FormFile("file")
	if e != nil {
		result(c, nil, ErrInput)
		return
	}
	defer f.Close()
	out, e := ctl.service.Upload(h.Filename, h.Size, f)
	result(c, out, e)
}

// Verify POST /api/attach/verify，返回当前用户的完成记录或可续传分片。
func (ctl *Controller) Verify(c *gin.Context) {
	var in VerifyRequest
	if e := decode(c, &in); e != nil {
		result(c, nil, e)
		return
	}
	out, e := ctl.service.Verify(user(c), in)
	result(c, out, e)
}

// Chunk POST /api/attach/chunk，query/form 的 chunkIndex 必须一致且从 0 开始。
func (ctl *Controller) Chunk(c *gin.Context) {
	defer cleanup(c)
	if e := parseMultipart(c); e != nil {
		result(c, nil, ErrInput)
		return
	}
	i, e := strconv.Atoi(c.Query("chunkIndex"))
	j, je := strconv.Atoi(c.PostForm("chunkIndex"))
	total, te := strconv.Atoi(c.Query("chunkTotal"))
	if e != nil || je != nil || te != nil || i != j {
		result(c, nil, ErrInput)
		return
	}
	f, _, e := c.Request.FormFile("chunk")
	if e != nil {
		result(c, nil, ErrInput)
		return
	}
	defer f.Close()
	e = ctl.service.Chunk(user(c), c.PostForm("uploadId"), c.PostForm("fileMd5"), c.PostForm("fileName"), i, total, f)
	result(c, nil, e)
}

// Merge POST /api/attach/merge，流式校验大小与 MD5 后发布成品。
func (ctl *Controller) Merge(c *gin.Context) {
	var in MergeRequest
	if e := decode(c, &in); e != nil {
		result(c, nil, e)
		return
	}
	out, e := ctl.service.Merge(user(c), in)
	result(c, out, e)
}
