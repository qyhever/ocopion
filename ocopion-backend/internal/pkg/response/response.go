package response

import "github.com/gin-gonic/gin"

// 全局状态码
const (
	CodeSuccess = 200
	CodeFail    = -1
)

// Response 统一接口返回结构体
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(200, Response{
		Code:    CodeSuccess,
		Message: "操作成功",
		Data:    data,
	})
}

// Fail 业务失败响应
func Fail(c *gin.Context, msg string) {
	c.JSON(200, Response{
		Code:    CodeFail,
		Message: msg,
	})
}

// FailWithCode 自定义错误码失败响应
func FailWithCode(c *gin.Context, code int, msg string) {
	c.JSON(200, Response{
		Code:    code,
		Message: msg,
	})
}
