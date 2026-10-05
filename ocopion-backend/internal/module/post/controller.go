package post

import (
	"errors"
	"strconv"

	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Controller struct{ service *Service }

func NewController(service *Service) *Controller { return &Controller{service: service} }

func positiveID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, ErrInvalidInput
	}
	return id, nil
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		response.Fail(c, "参数不合法")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Fail(c, "文章不存在")
	default:
		response.Fail(c, "操作失败")
	}
}

func (ctl *Controller) Create(c *gin.Context) {
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	post, err := ctl.service.Create(c.Request.Context(), input.Title, input.Content)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, post)
}

func (ctl *Controller) Update(c *gin.Context) {
	id, err := positiveID(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	var input struct {
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	post, err := ctl.service.Update(c.Request.Context(), id, input.Title, input.Content)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, post)
}

func (ctl *Controller) Get(c *gin.Context) {
	id, err := positiveID(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	post, err := ctl.service.Get(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, post)
}

func (ctl *Controller) Delete(c *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	if err := ctl.service.Delete(c.Request.Context(), input.IDs); err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, nil)
}

func queryInt(c *gin.Context, key string) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return 0
	}
	return value
}

func (ctl *Controller) List(c *gin.Context) {
	result, err := ctl.service.List(c.Request.Context(), queryInt(c, "pageNum"), queryInt(c, "pageSize"), c.Query("keyword"))
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, result)
}
