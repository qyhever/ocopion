package user

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
	case errors.Is(err, ErrEmailExists):
		response.Fail(c, "邮箱已存在")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Fail(c, "用户不存在")
	default:
		response.Fail(c, "操作失败")
	}
}

func (ctl *Controller) Create(c *gin.Context) {
	var input CreateRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	user, err := ctl.service.Create(c.Request.Context(), input)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, newUserResponse(*user))
}

func (ctl *Controller) Update(c *gin.Context) {
	id, err := positiveID(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	input := UpdateRequest{ID: id}
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	user, err := ctl.service.Update(c.Request.Context(), input.ID, input)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, newUserResponse(*user))
}

func (ctl *Controller) Get(c *gin.Context) {
	id, err := positiveID(c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	input := GetRequest{ID: id}
	user, err := ctl.service.Get(c.Request.Context(), input.ID)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, newUserResponse(*user))
}

func (ctl *Controller) Delete(c *gin.Context) {
	var input DeleteRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	if err := ctl.service.Delete(c.Request.Context(), input.IDs); err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, (*DeleteResponse)(nil))
}

func queryInt(c *gin.Context, key string) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return 0
	}
	return value
}

func (ctl *Controller) List(c *gin.Context) {
	input := ListRequest{PageNum: queryInt(c, "pageNum"), PageSize: queryInt(c, "pageSize"), Keyword: c.Query("keyword")}
	result, err := ctl.service.List(c.Request.Context(), input.PageNum, input.PageSize, input.Keyword)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, newListResponse(result))
}
