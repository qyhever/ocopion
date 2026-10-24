package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"ocopion/internal/config"
	jwtpkg "ocopion/internal/pkg/jwt"
)

func TestAuthenticationSources(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "test-secret"}, Auth: config.AuthConfig{Whitelist: []string{"/protected", "*:/protected"}}}
	access, refresh, err := jwtpkg.GenToken(42)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Any("/protected", JWTAuthMiddleware(), func(c *gin.Context) {
		if c.Request.Method != "OPTIONS" {
			if id, ok := c.Get(ContextUserIDKey); !ok || id != uint64(42) {
				t.Errorf("wrong identity: %v", id)
			}
		}
		c.Status(204)
	})
	for _, tc := range []struct {
		name, method, header, query, cookie string
		status                              int
	}{
		{"missing token despite whitelist", "GET", "", "", "", 401},
		{"bearer", "GET", "Bearer " + access, "", "", 204},
		{"raw authorization", "GET", access, "", "", 204},
		{"query", "GET", "", access, "", 204},
		{"cookie", "GET", "", "", access, 204},
		{"header has priority", "GET", "invalid", access, access, 401},
		{"query has priority", "GET", "", "invalid", access, 401},
		{"refresh rejected", "GET", "Bearer " + refresh, "", "", 401},
		{"options bypass", "OPTIONS", "invalid", "", "", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/protected?token="+tc.query, nil)
			req.Header.Set("Authorization", tc.header)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "token", Value: tc.cookie})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
