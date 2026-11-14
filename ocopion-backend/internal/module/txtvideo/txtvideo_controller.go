package txtvideo

import (
	"errors"
	"net/http"

	"ocopion/internal/middleware"
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type Controller struct{ service *Service }

func NewController(service *Service) *Controller { return &Controller{service: service} }
func owner(c *gin.Context) uint64 {
	v, _ := c.Get(middleware.ContextUserIDKey)
	id, _ := v.(uint64)
	return id
}
func (ctl *Controller) Create(c *gin.Context) {
	if owner(c) == 0 {
		response.Unauthorized(c, "请先登录")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxCoverSize+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
		response.Fail(c, "上传失败：请提交 multipart 表单，封面不能超过 10 MiB")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	header, err := c.FormFile("cover")
	if err != nil {
		response.Fail(c, "请选择封面图片")
		return
	}
	f, err := header.Open()
	if err != nil {
		response.Fail(c, "读取封面失败")
		return
	}
	defer f.Close()
	cover, err := ReadCover(f)
	if err != nil {
		response.Fail(c, err.Error())
		return
	}
	task, err := ctl.service.Create(owner(c), c.PostForm("text"), c.PostForm("speed"), cover)
	if err != nil {
		response.Fail(c, err.Error())
		return
	}
	response.Success(c, task)
}
func (ctl *Controller) Get(c *gin.Context) {
	if owner(c) == 0 {
		response.Unauthorized(c, "请先登录")
		return
	}
	task, err := ctl.service.Get(owner(c), c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			response.FailWithCode(c, 404, err.Error())
			return
		}
		response.Fail(c, err.Error())
		return
	}
	response.Success(c, task)
}
func (ctl *Controller) File(c *gin.Context) {
	f, err := ctl.service.Open(c.Param("filename"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Type", "video/mp4")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Query("download") == "1" {
		c.Header("Content-Disposition", `attachment; filename="`+info.Name()+`"`)
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), f)
}
