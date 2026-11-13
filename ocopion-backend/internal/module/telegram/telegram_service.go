package telegram

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"
)

var (
	ErrInvalidInput = errors.New("参数不合法")
	ErrSendFailed   = errors.New("Telegram 消息发送失败")
)

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Send(ctx context.Context, input SendRequest) error {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return ErrInvalidInput
	}
	if err := s.repo.Send(ctx, text); err != nil {
		zap.L().Error("Telegram 消息发送失败", zap.Error(err))
		return ErrSendFailed
	}
	return nil
}
