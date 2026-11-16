package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ocopion/internal/config"
	"ocopion/internal/middleware"
	"ocopion/internal/module/user"
	jwtpkg "ocopion/internal/pkg/jwt"
	"ocopion/internal/pkg/password"
	"ocopion/internal/pkg/response"

	jwtlib "github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
)

// Embedding a nil Repository makes every unexpected repository call fail immediately.
type forbiddenRepository struct{ user.Repository }

func TestMockAuthentication(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{
		JWT: config.JWTConfig{Secret: "mock-test-secret"},
		Auth: config.AuthConfig{MockUser: config.MockUserConfig{
			Enabled: true, Email: "mock@example.com", Username: "mock-admin", Password: "mock-secret",
		}},
	}
	gin.SetMode(gin.TestMode)
	repo := forbiddenRepository{}
	ctl := NewAuthController(NewAuthService(repo))
	userCtl := user.NewController(user.NewService(repo))
	r := gin.New()
	r.POST("/api/auth/login", ctl.Login)
	r.POST("/api/auth/refresh", ctl.Refresh)
	r.GET("/api/user/me", middleware.RequiredJWTAuthMiddleware(), userCtl.Me)

	call := func(method, path, body, token string, status, code int) response.Response {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var result response.Response
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != status || result.Code != code {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		if strings.Contains(strings.ToLower(w.Body.String()), "password") || strings.Contains(w.Body.String(), "mock-secret") {
			t.Fatal("password leaked")
		}
		return result
	}
	tokens := dataMap(t, call("POST", "/api/auth/login", `{"email":"mock@example.com","password":"mock-secret"}`, "", 200, 200))
	if len(tokens) != 2 {
		t.Fatalf("unexpected token response: %#v", tokens)
	}
	access, refresh := tokens["accessToken"].(string), tokens["refreshToken"].(string)
	for _, input := range []LoginRequest{
		{Email: "other@example.com", Password: "mock-secret"},
		{Email: "mock@example.com", Password: "wrong"},
		{Email: "Mock@example.com", Password: "mock-secret"},
		{Email: "mock@example.com", Password: "mock-secret "},
	} {
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		if got := call("POST", "/api/auth/login", string(body), "", 200, -1); got.Message != ErrCredentials.Error() {
			t.Fatal(got)
		}
	}
	for _, body := range []string{
		`{}`, `{`, `null`, `{"email":2}`,
		`{"email":"bad","password":"mock-secret"}`,
		`{"email":"mock@example.com","password":""}`,
		`{"email":"mock@example.com","password":" "}`,
		`{"email":"mock@example.com","password":"` + strings.Repeat("a", 73) + `"}`,
	} {
		if got := call("POST", "/api/auth/login", body, "", 200, -1); got.Message != ErrInvalidInput.Error() {
			t.Fatal(got)
		}
	}
	me := dataMap(t, call("GET", "/api/user/me?id=2", "", access, 200, 200))
	want := map[string]interface{}{
		"id": float64(1), "email": "mock@example.com", "username": "mock-admin", "nickname": "mock-admin",
		"status": "active", "phone": "", "avatarUrl": "", "createdAt": "0001-01-01T00:00:00Z", "updatedAt": "0001-01-01T00:00:00Z",
	}
	if len(me) != len(want) {
		t.Fatalf("unexpected user fields: %#v", me)
	}
	for key, value := range want {
		if me[key] != value {
			t.Errorf("%s = %v, want %v", key, me[key], value)
		}
	}
	rotated := dataMap(t, call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, refresh), "", 200, 200))
	if len(rotated) != 2 || rotated["accessToken"] == access || rotated["refreshToken"] == refresh {
		t.Fatal("refresh did not issue new mock tokens")
	}
	call("GET", "/api/user/me", "", rotated["accessToken"].(string), 200, 200)
	for _, body := range []string{`{}`, `{`, `null`, `{"refreshToken":2}`, `{"refreshToken":" "}`} {
		call("POST", "/api/auth/refresh", body, "", 200, -1)
	}
	sign := func(kind string, id uint64, expires time.Time) string {
		t.Helper()
		token, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtpkg.MyClaims{
			UserID: id, TokenType: kind, StandardClaims: jwtlib.StandardClaims{ExpiresAt: expires.Unix()},
		}).SignedString([]byte("mock-test-secret"))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	expired := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	for _, token := range []string{
		"", "invalid", refresh, access + "tampered",
		sign(jwtpkg.TokenTypeAccess, 1, expired), sign(jwtpkg.TokenTypeAccess, 2, future), sign("other", 1, future),
	} {
		call("GET", "/api/user/me", "", token, 401, 1006)
	}
	for _, token := range []string{
		"invalid", access, refresh + "tampered",
		sign(jwtpkg.TokenTypeRefresh, 1, expired), sign(jwtpkg.TokenTypeRefresh, 2, future), sign("other", 1, future),
	} {
		call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, token), "", 401, 1006)
	}
}

func TestDatabaseAuthenticationFallback(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	hash, err := password.Hash("database-secret")
	if err != nil {
		t.Fatal(err)
	}
	repo := &authUserRepository{users: map[int64]user.User{
		7: {ID: 7, Email: "database@example.com", PasswordHash: hash, Username: "database-user", Status: "active"},
	}}
	ctx := context.Background()
	for _, initialized := range []bool{false, true} {
		t.Run(fmt.Sprintf("initialized=%t", initialized), func(t *testing.T) {
			config.GlobalConfig = nil
			if initialized {
				config.GlobalConfig = &config.Config{
					JWT: config.JWTConfig{Secret: "fallback-secret"},
					Auth: config.AuthConfig{MockUser: config.MockUserConfig{
						Enabled: false, Email: "mock@example.com", Password: "mock-secret",
					}},
				}
			}
			s := NewAuthService(repo)
			// A wrong password must be checked against the stored hash, even without config.
			if _, err := s.Login(ctx, LoginRequest{Email: "database@example.com", Password: "wrong"}); !errors.Is(err, ErrCredentials) {
				t.Fatalf("database credentials not checked: %v", err)
			}
			got, err := user.NewService(repo).Me(ctx, 7)
			if err != nil || got.ID != 7 || got.Username != "database-user" {
				t.Fatalf("database user not returned: %#v %v", got, err)
			}
			if !initialized {
				return // JWT signing still requires initialized configuration.
			}
			tokens, err := s.Login(ctx, LoginRequest{Email: "database@example.com", Password: "database-secret"})
			if err != nil || tokens.UserID != 7 {
				t.Fatalf("database login: %#v %v", tokens, err)
			}
			if refreshed, err := s.Refresh(ctx, RefreshRequest{RefreshToken: tokens.RefreshToken}); err != nil || refreshed.UserID != 7 {
				t.Fatalf("database refresh: %#v %v", refreshed, err)
			}
		})
	}
}
