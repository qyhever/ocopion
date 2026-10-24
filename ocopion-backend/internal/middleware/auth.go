package middleware

import (
	"strings"

	jwtpkg "ocopion/internal/pkg/jwt"
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserIDKey = "userID"
)

// JWTAuthMiddleware 基于JWT的认证中间件
func JWTAuthMiddleware() func(c *gin.Context) {
	// 从请求中读取 Authorization 或 token，校验并解析 JWT。
	// 解析成功后，将用户ID写入上下文：key 为 ContextUserIDKey。
	// 若未携带 token 或 token 无效，则返回统一错误响应并中止后续处理。
	return func(c *gin.Context) {
		// 0) 跳过 CORS 预检
		if c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		// 1) 优先从 Authorization 头读取，支持 "Bearer <token>" 格式
		authHeader := c.GetHeader("Authorization")
		var token string
		if authHeader != "" {
			if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				token = strings.TrimSpace(authHeader[7:])
			} else {
				// 非 Bearer 直接作为 token 尝试解析
				token = strings.TrimSpace(authHeader)
			}
		}

		// 2) 其次尝试从查询参数读取
		if token == "" {
			token = strings.TrimSpace(c.Query("token"))
		}
		// 3) 再次尝试从 Cookie 读取
		if token == "" {
			if ck, err := c.Cookie("token"); err == nil {
				token = strings.TrimSpace(ck)
			}
		}

		if token == "" {
			response.Unauthorized(c, "未登录或登录过期")
			return
		}

		// 解析并验证 token
		claims, err := jwtpkg.ParseToken(token)
		if err != nil || claims == nil {
			response.Unauthorized(c, "登录状态失效")
			return
		}
		if !claims.IsAccessToken() {
			response.Unauthorized(c, "登录状态失效")
			return
		}

		// 将用户ID写入上下文，供后续处理使用
		c.Set(ContextUserIDKey, claims.UserID)
		c.Next()
	}
}
