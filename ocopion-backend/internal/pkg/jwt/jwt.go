package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"time"

	"ocopion/internal/config"

	"github.com/dgrijalva/jwt-go"
)

type MyClaims struct {
	UserID    uint64 `json:"user_id"`
	TokenType string `json:"token_type"`
	jwt.StandardClaims
}

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

func getSecret() ([]byte, error) {
	cfg := config.GetConfig()
	if cfg == nil || cfg.JWT.Secret == "" {
		return nil, errors.New("JWT secret is not configured")
	}
	return []byte(cfg.JWT.Secret), nil
}

func keyFunc(token *jwt.Token) (interface{}, error) {
	if token.Method != jwt.SigningMethodHS256 {
		return nil, errors.New("invalid signing method")
	}
	return getSecret()
}

func parseDurationOrFallback(raw string, fallback time.Duration) time.Duration {
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return duration
}

// GenToken 为指定用户签发独立的 access 和 refresh token。
func GenToken(userID uint64) (aToken, rToken string, err error) {
	secret, err := getSecret()
	if err != nil {
		return "", "", err
	}
	if userID == 0 || userID > math.MaxInt64 {
		return "", "", errors.New("invalid user ID")
	}
	cfg := config.GetConfig()
	accessDuration := parseDurationOrFallback(cfg.JWT.AccessExpiresIn, 8*time.Hour)
	refreshDuration := parseDurationOrFallback(cfg.JWT.RefreshExpiresIn, 72*time.Hour)
	if accessDuration <= 0 || refreshDuration <= 0 {
		return "", "", errors.New("JWT duration must be positive")
	}
	sign := func(kind string, duration time.Duration) (string, error) {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			return "", err
		}
		claims := MyClaims{UserID: userID, TokenType: kind, StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(duration).Unix(), Issuer: "jeve", Id: hex.EncodeToString(id),
		}}
		return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	}
	aToken, err = sign(TokenTypeAccess, accessDuration)
	if err != nil {
		return "", "", err
	}
	rToken, err = sign(TokenTypeRefresh, refreshDuration)
	if err != nil {
		return "", "", err
	}
	return aToken, rToken, nil
}

// ParseToken 验证签名、有效期、类型和用户 ID。
func ParseToken(tokenString string) (*MyClaims, error) {
	claims := new(MyClaims)
	token, err := jwt.ParseWithClaims(tokenString, claims, keyFunc)
	if err != nil {
		return nil, err
	}
	if !token.Valid || claims.ExpiresAt <= time.Now().Unix() || claims.UserID == 0 || claims.UserID > math.MaxInt64 ||
		(!claims.IsAccessToken() && !claims.IsRefreshToken()) {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (c *MyClaims) IsAccessToken() bool  { return c != nil && c.TokenType == TokenTypeAccess }
func (c *MyClaims) IsRefreshToken() bool { return c != nil && c.TokenType == TokenTypeRefresh }
