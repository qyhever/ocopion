package jwt

import (
	"math"
	"testing"
	"time"

	jwtlib "github.com/dgrijalva/jwt-go"
	"ocopion/internal/config"
)

func TestTokenValidation(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "test-secret"}}
	a, r, err := GenToken(1)
	if err != nil {
		t.Fatal(err)
	}
	for token, kind := range map[string]string{a: TokenTypeAccess, r: TokenTypeRefresh} {
		claims, err := ParseToken(token)
		if err != nil || claims.UserID != 1 || claims.TokenType != kind || claims.Id == "" {
			t.Fatalf("claims: %#v %v", claims, err)
		}
	}
	a2, r2, err := GenToken(1)
	if err != nil || a == a2 || r == r2 {
		t.Fatal("tokens must have unique IDs")
	}
	for _, tc := range []struct {
		name   string
		id     uint64
		kind   string
		expiry int64
		method jwtlib.SigningMethod
		secret string
	}{
		{"expired", 1, TokenTypeAccess, time.Now().Add(-time.Hour).Unix(), jwtlib.SigningMethodHS256, "test-secret"},
		{"missing expiry", 1, TokenTypeAccess, 0, jwtlib.SigningMethodHS256, "test-secret"},
		{"wrong signature", 1, TokenTypeAccess, time.Now().Add(time.Hour).Unix(), jwtlib.SigningMethodHS256, "wrong"},
		{"wrong algorithm", 1, TokenTypeAccess, time.Now().Add(time.Hour).Unix(), jwtlib.SigningMethodHS384, "test-secret"},
		{"zero user", 0, TokenTypeAccess, time.Now().Add(time.Hour).Unix(), jwtlib.SigningMethodHS256, "test-secret"},
		{"overflow user", math.MaxUint64, TokenTypeAccess, time.Now().Add(time.Hour).Unix(), jwtlib.SigningMethodHS256, "test-secret"},
		{"wrong type", 1, "other", time.Now().Add(time.Hour).Unix(), jwtlib.SigningMethodHS256, "test-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, err := jwtlib.NewWithClaims(tc.method, MyClaims{UserID: tc.id, TokenType: tc.kind, StandardClaims: jwtlib.StandardClaims{ExpiresAt: tc.expiry}}).SignedString([]byte(tc.secret))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseToken(token); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	for _, id := range []uint64{0, math.MaxUint64} {
		if _, _, err := GenToken(id); err == nil {
			t.Fatal("invalid ID accepted")
		}
	}
	for _, duration := range []string{"0s", "-1h"} {
		config.GlobalConfig.JWT.AccessExpiresIn = duration
		if _, _, err := GenToken(1); err == nil {
			t.Fatal("non-positive access duration accepted")
		}
		config.GlobalConfig.JWT.AccessExpiresIn = "1h"
		config.GlobalConfig.JWT.RefreshExpiresIn = duration
		if _, _, err := GenToken(1); err == nil {
			t.Fatal("non-positive refresh duration accepted")
		}
	}
	for _, cfg := range []*config.Config{nil, {}} {
		config.GlobalConfig = cfg
		if _, _, err := GenToken(1); err == nil {
			t.Fatal("missing config accepted")
		}
		if _, err := ParseToken(a); err == nil {
			t.Fatal("missing secret accepted")
		}
	}
}

func TestDurationFallback(t *testing.T) {
	for _, raw := range []string{"", "invalid"} {
		if parseDurationOrFallback(raw, time.Hour) != time.Hour {
			t.Fatal("missing fallback")
		}
	}
	if parseDurationOrFallback("168h", time.Hour) != 7*24*time.Hour {
		t.Fatal("incorrect duration")
	}
}
