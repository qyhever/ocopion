package auth

import (
	"errors"

	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AuthController struct{ service *AuthService }

func NewAuthController(service *AuthService) *AuthController {
	return &AuthController{service: service}
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrUnauthorized):
		response.Unauthorized(c, ErrUnauthorized.Error())
	case errors.Is(err, ErrInvalidInput):
		response.Fail(c, ErrInvalidInput.Error())
	case errors.Is(err, ErrCredentials):
		response.Fail(c, ErrCredentials.Error())
	default:
		response.Fail(c, "操作失败")
	}
}

func (ctl *AuthController) Login(c *gin.Context) {
	var input LoginRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	tokens, err := ctl.service.Login(c.Request.Context(), input)
	if err != nil {
		respondError(c, err)
		return
	}
	zap.L().Info("登录成功",
		zap.Int64("user_id", tokens.UserID),
		zap.String("email", input.Email),
		zap.String("ip", c.ClientIP()),
		zap.String("user-agent", c.Request.UserAgent()),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
	)
	response.Success(c, tokens)
}

func (ctl *AuthController) Refresh(c *gin.Context) {
	var input RefreshRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, ErrInvalidInput)
		return
	}
	tokens, err := ctl.service.Refresh(c.Request.Context(), input)
	if err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, tokens)
}
