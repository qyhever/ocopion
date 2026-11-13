package mail

import (
	"errors"
	"net/mail"
	"strings"

	"go.uber.org/zap"
)

var (
	ErrInvalidInput = errors.New("参数不合法")
	ErrSendFailed   = errors.New("邮件发送失败")
)

type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Send(input SendRequest) error {
	if strings.ContainsAny(input.To, "\r\n") || strings.ContainsAny(input.Subject, "\r\n") {
		return ErrInvalidInput
	}
	input.To = strings.TrimSpace(input.To)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Body = strings.TrimSpace(input.Body)
	if input.To == "" || input.Subject == "" || input.Body == "" {
		return ErrInvalidInput
	}
	address, err := mail.ParseAddress(input.To)
	if err != nil || address.Address != input.To {
		return ErrInvalidInput
	}
	if err := s.repo.Send(input.To, input.Subject, input.Body); err != nil {
		zap.L().Error("邮件发送失败", zap.Error(err))
		return ErrSendFailed
	}
	return nil
}
