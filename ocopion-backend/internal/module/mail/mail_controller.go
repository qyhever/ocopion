package mail

import (
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type Controller struct{ service *Service }

func NewController(service *Service) *Controller { return &Controller{service: service} }

func (ctl *Controller) Send(c *gin.Context) {
	var input SendRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		response.Fail(c, ErrInvalidInput.Error())
		return
	}
	if err := ctl.service.Send(input); err != nil {
		response.Fail(c, err.Error())
		return
	}
	response.Success(c, nil)
}
