package auth

import (
	"context"
	"encoding/json"
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
	"gorm.io/gorm"
)

func TestAuthentication(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "test-secret"}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	repo := &authUserRepository{users: make(map[int64]user.User)}
	ctl := NewAuthController(NewAuthService(repo))
	userCtl := user.NewController(user.NewService(repo))
	r.POST("/api/auth/login", ctl.Login)
	r.POST("/api/auth/refresh", ctl.Refresh)
	r.GET("/api/user/me", middleware.JWTAuthMiddleware(), userCtl.Me)
	r.GET("/api/user/:id", userCtl.Get)
	hash, err := password.Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	repo.users[1] = user.User{ID: 1, Email: "one@example.com", PasswordHash: hash, Status: "active"}
	repo.users[2] = user.User{ID: 2, Email: "two@example.com", PasswordHash: hash, Status: "disabled"}
	repo.users[3] = user.User{ID: 3, Email: "deleted@example.com", PasswordHash: hash, Status: "active"}
	deleted := repo.users[3]
	deleted.DeletedAt.Valid = true
	repo.users[3] = deleted
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
		if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), hash) {
			t.Fatal("password leaked")
		}
		return result
	}
	tokens := dataMap(t, call("POST", "/api/auth/login", `{"email":"one@example.com","password":"secret"}`, "", 200, 200))
	access, refresh := tokens["accessToken"].(string), tokens["refreshToken"].(string)
	for _, tc := range []struct{ email, password string }{
		{"one@example.com", "wrong"}, {"absent@example.com", "secret"}, {"two@example.com", "secret"}, {"deleted@example.com", "secret"},
	} {
		got := call("POST", "/api/auth/login", fmt.Sprintf(`{"email":%q,"password":%q}`, tc.email, tc.password), "", 200, -1)
		if got.Message != "邮箱或密码错误" {
			t.Fatal(got)
		}
	}
	for _, body := range []string{`{}`, `{`, `null`, `{"email":2}`, `{"email":"bad","password":"secret"}`, `{"email":"one@example.com","password":""}`, `{"email":"one@example.com","password":"` + strings.Repeat("a", 73) + `"}`} {
		call("POST", "/api/auth/login", body, "", 200, -1)
	}
	seen := map[string]bool{access: true, refresh: true}
	for i := 0; i < 2; i++ {
		got := dataMap(t, call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, refresh), "", 200, 200))
		for _, key := range []string{"accessToken", "refreshToken"} {
			token := got[key].(string)
			if seen[token] {
				t.Fatal("refresh did not rotate tokens")
			}
			seen[token] = true
		}
	}
	for _, body := range []string{`{}`, `{`, `{"refreshToken":2}`} {
		call("POST", "/api/auth/refresh", body, "", 200, -1)
	}
	for _, token := range []string{access, "invalid", refresh + "tampered"} {
		call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, token), "", 401, 1006)
	}
	expired, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtpkg.MyClaims{UserID: 1, TokenType: jwtpkg.TokenTypeRefresh, StandardClaims: jwtlib.StandardClaims{ExpiresAt: time.Now().Add(-time.Hour).Unix()}}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, expired), "", 401, 1006)
	me := dataMap(t, call("GET", "/api/user/me?id=2", "", access, 200, 200))
	if me["id"] != float64(1) || len(me) != 9 {
		t.Fatal(me)
	}
	call("GET", "/api/user/2", "", "", 200, 200)
	for _, token := range []string{"", refresh, "invalid", access + "tampered"} {
		call("GET", "/api/user/me", "", token, 401, 1006)
	}
	for _, id := range []uint64{2, 3, 999} {
		a, r, err := jwtpkg.GenToken(id)
		if err != nil {
			t.Fatal(err)
		}
		call("GET", "/api/user/me", "", a, 401, 1006)
		call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, r), "", 401, 1006)
	}
	u := repo.users[1]
	u.Status = "disabled"
	repo.users[1] = u
	call("GET", "/api/user/me", "", access, 401, 1006)
	call("POST", "/api/auth/refresh", fmt.Sprintf(`{"refreshToken":%q}`, refresh), "", 401, 1006)
}

// Only the existing user repository reads are needed for authentication.
type authUserRepository struct {
	user.Repository
	users map[int64]user.User
}

func (r *authUserRepository) Get(_ context.Context, id int64) (*user.User, error) {
	u, ok := r.users[id]
	if !ok || u.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return &u, nil
}

func (r *authUserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	for _, u := range r.users {
		if u.Email == email {
			return r.Get(ctx, u.ID)
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func dataMap(t *testing.T, result response.Response) map[string]interface{} {
	t.Helper()
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data: %#v", result.Data)
	}
	return data
}
