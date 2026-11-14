package router

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ocopion/internal/config"
	"ocopion/internal/module/telegram"
	jwtpkg "ocopion/internal/pkg/jwt"
	"ocopion/internal/pkg/response"

	"github.com/dgrijalva/jwt-go"
)

type telegramRepository struct{ calls int }

func (r *telegramRepository) Send(context.Context, string) error { r.calls++; return nil }

func TestTelegramRouterAuthentication(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "tg-test-secret"}}
	access, refresh, err := jwtpkg.GenToken(999)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtpkg.MyClaims{
		UserID: 999, TokenType: jwtpkg.TokenTypeAccess,
		StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(-time.Minute).Unix()},
	}).SignedString([]byte(config.GlobalConfig.JWT.Secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, whitelist := range [][]string{nil, {"/api/telegram"}, {"POST:/api/telegram"}, {"*:/api/telegram"}} {
		config.GlobalConfig.Auth.Whitelist = whitelist
		repo := &telegramRepository{}
		r := newTestRouter(t, nil, nil, nil, nil, nil, nil, telegram.NewController(telegram.NewService(repo)))
		for _, token := range []string{"", "invalid-token", expired, refresh, access} {
			req := httptest.NewRequest("POST", "/api/telegram", strings.NewReader(`{"text":"hello"}`))
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			before := repo.calls
			r.ServeHTTP(w, req)
			var result response.Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if token == access || len(whitelist) > 0 {
				if w.Code != 200 || result.Code != 200 || repo.calls != before+1 {
					t.Fatalf("authorized whitelist=%v: %s", whitelist, w.Body.String())
				}
			} else if w.Code != 401 || result.Code != 1006 || repo.calls != before {
				t.Fatalf("whitelist=%v: %s", whitelist, w.Body.String())
			}
		}
	}
}
