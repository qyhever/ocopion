package post

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"ocopion/internal/pkg/pagination"
)

var ErrInvalidInput = errors.New("参数不合法")

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func validTitle(title string) bool {
	return strings.TrimSpace(title) != "" && utf8.RuneCountInString(title) <= 128
}

func (s *Service) Create(ctx context.Context, title, content string) (*Post, error) {
	if !validTitle(title) || strings.TrimSpace(content) == "" {
		return nil, ErrInvalidInput
	}
	post := &Post{Title: title, Content: content}
	if err := s.repo.Create(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *Service) Update(ctx context.Context, id int64, title, content *string) (*Post, error) {
	if id <= 0 || title == nil && content == nil {
		return nil, ErrInvalidInput
	}
	fields := make(map[string]interface{})
	if title != nil {
		if !validTitle(*title) {
			return nil, ErrInvalidInput
		}
		fields["title"] = *title
	}
	if content != nil {
		if strings.TrimSpace(*content) == "" {
			return nil, ErrInvalidInput
		}
		fields["content"] = *content
	}
	return s.repo.Update(ctx, id, fields)
}

func (s *Service) Get(ctx context.Context, id int64) (*Post, error) {
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
	posts, total, err := s.repo.List(ctx, params, strings.TrimSpace(keyword))
	return ListResult{List: posts, PageNum: params.PageNum, PageSize: params.PageSize, Total: total}, err
}
