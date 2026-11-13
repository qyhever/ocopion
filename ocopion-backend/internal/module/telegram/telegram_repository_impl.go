package telegram

import (
	"context"

	"ocopion/internal/pkg/telegram"
)

type ClientRepository struct{ client *telegram.Client }

func NewClientRepository(client *telegram.Client) *ClientRepository {
	return &ClientRepository{client: client}
}

func (r *ClientRepository) Send(ctx context.Context, text string) error {
	return r.client.SendMessage(ctx, text)
}
