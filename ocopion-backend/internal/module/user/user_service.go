package user

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"ocopion/internal/pkg/pagination"
	"ocopion/internal/pkg/password"

	"gorm.io/gorm"
)

var (
	ErrInvalidInput = errors.New("参数不合法")
	ErrEmailExists  = errors.New("邮箱已存在")
	ErrUnauthorized = errors.New("未登录或登录过期")
)

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func validString(value string, limit int, required bool) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit && (!required || strings.TrimSpace(value) != "")
}
func validEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return validString(value, 128, true) && err == nil && address.Address == value
}
func validPassword(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 72
}

func (s *Service) Create(ctx context.Context, input CreateRequest) (*User, error) {
	status := "active"
	if input.Status != nil {
		status = *input.Status
	}
	if !validEmail(input.Email) || !validPassword(input.Password) ||
		!validString(input.Username, 128, true) || !validString(input.Nickname, 128, true) ||
		!validString(input.Phone, 32, false) || !validString(input.AvatarURL, 512, false) || !validString(status, 32, true) {
		return nil, ErrInvalidInput
	}
	hash, err := password.Hash(input.Password)
	if err != nil {
		return nil, err
	}
	user := &User{Email: input.Email, Phone: input.Phone, PasswordHash: hash, Username: input.Username,
		Nickname: input.Nickname, AvatarURL: input.AvatarURL, Status: status}
	if err := s.repo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateRequest) (*User, error) {
	if id <= 0 {
		return nil, ErrInvalidInput
	}
	fields := make(map[string]interface{})
	if input.Email != nil {
		if !validEmail(*input.Email) {
			return nil, ErrInvalidInput
		}
		fields["email"] = *input.Email
	}
	for _, field := range []struct {
		name     string
		value    *string
		limit    int
		required bool
	}{
		{"phone", input.Phone, 32, false}, {"username", input.Username, 128, true},
		{"nickname", input.Nickname, 128, true}, {"avatar_url", input.AvatarURL, 512, false}, {"status", input.Status, 32, true},
	} {
		if field.value != nil {
			if !validString(*field.value, field.limit, field.required) {
				return nil, ErrInvalidInput
			}
			fields[field.name] = *field.value
		}
	}
	if input.Password != nil {
		if !validPassword(*input.Password) {
			return nil, ErrInvalidInput
		}
		hash, err := password.Hash(*input.Password)
		if err != nil {
			return nil, err
		}
		fields["password_hash"] = hash
	}
	if len(fields) == 0 {
		return nil, ErrInvalidInput
	}
	return s.repo.Update(ctx, id, fields)
}

func (s *Service) Get(ctx context.Context, id int64) (*User, error) {
	if id <= 0 {
		return nil, ErrInvalidInput
	}
	return s.repo.Get(ctx, id)
}
func (s *Service) Delete(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return ErrInvalidInput
	}
	for _, id := range ids {
		if id <= 0 {
			return ErrInvalidInput
		}
	}
	return s.repo.Delete(ctx, ids)
}
func (s *Service) List(ctx context.Context, page, pageSize int, keyword string) (ListResult, error) {
	params := pagination.Normalize(page, pageSize)
	users, total, err := s.repo.List(ctx, params, strings.TrimSpace(keyword))
	return ListResult{List: users, PageNum: params.PageNum, PageSize: params.PageSize, Total: total}, err
}

func (s *Service) Me(ctx context.Context, id int64) (*User, error) {
	if id <= 0 {
		return nil, ErrUnauthorized
	}
	user, err := s.repo.Get(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if !user.IsActive() {
		return nil, ErrUnauthorized
	}
	return user, nil
}
