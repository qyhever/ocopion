package user

import (
	"context"

	"ocopion/internal/pkg/pagination"
)

type Repository interface {
	Create(context.Context, *User) error
	Get(context.Context, int64) (*User, error)
	GetByEmail(context.Context, string) (*User, error)
	Update(context.Context, int64, map[string]interface{}) (*User, error)
	Delete(context.Context, []int64) error
	List(context.Context, pagination.Params, string) ([]User, int64, error)
}
