package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"ocopion/internal/module/user"
	jwtpkg "ocopion/internal/pkg/jwt"
	"ocopion/internal/pkg/password"

	"gorm.io/gorm"
)

var (
	ErrInvalidInput = errors.New("参数不合法")
	ErrCredentials  = errors.New("邮箱或密码错误")
	ErrUnauthorized = errors.New("未登录或登录过期")
)

type AuthService struct{ repo user.Repository }

func NewAuthService(repo user.Repository) *AuthService { return &AuthService{repo: repo} }

func validEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 128 && err == nil && address.Address == value
}

func validPassword(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 72
}

func issueTokens(id int64) (*TokenResponse, error) {
	access, refresh, err := jwtpkg.GenToken(uint64(id))
	if err != nil {
		return nil, err
	}
	return &TokenResponse{AccessToken: access, RefreshToken: refresh}, nil
}

func (s *AuthService) Login(ctx context.Context, input LoginRequest) (*TokenResponse, error) {
	if !validEmail(input.Email) || !validPassword(input.Password) {
		return nil, ErrInvalidInput
	}
	user, err := s.repo.GetByEmail(ctx, input.Email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCredentials
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive() || password.Compare(user.PasswordHash, input.Password) != nil {
		return nil, ErrCredentials
	}
	return issueTokens(user.ID)
}

func (s *AuthService) Refresh(ctx context.Context, input RefreshRequest) (*TokenResponse, error) {
	if strings.TrimSpace(input.RefreshToken) == "" {
		return nil, ErrInvalidInput
	}
	claims, err := jwtpkg.ParseToken(input.RefreshToken)
	if err != nil || !claims.IsRefreshToken() {
		return nil, ErrUnauthorized
	}
	user, err := s.repo.Get(ctx, int64(claims.UserID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive() {
		return nil, ErrUnauthorized
	}
	return issueTokens(user.ID)
}
