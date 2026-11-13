package telegram

import "context"

type Repository interface {
	Send(ctx context.Context, text string) error
}
