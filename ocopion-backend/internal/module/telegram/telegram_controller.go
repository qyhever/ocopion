package telegram

import (
	"encoding/json"
	"io"

	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type Controller struct{ service *Service }

func NewController(service *Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) Send(c *gin.Context) {
	var input SendRequest
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&input); err != nil {
		response.Fail(c, ErrInvalidInput.Error())
		return
	}
	// 一个请求只接受一个完整 JSON 对象，拒绝尾随 JSON 或非法字符。
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		response.Fail(c, ErrInvalidInput.Error())
		return
	}
	if err := ctl.service.Send(c.Request.Context(), input); err != nil {
		response.Fail(c, err.Error())
		return
	}
	response.Success(c, nil)
}
