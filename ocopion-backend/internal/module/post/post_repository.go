package post

import (
	"context"

	"ocopion/internal/pkg/pagination"
)

type Repository interface {
	Create(context.Context, *Post) error
	Get(context.Context, int64) (*Post, error)
	Update(context.Context, int64, map[string]interface{}) (*Post, error)
	Delete(context.Context, []int64) error
	List(context.Context, pagination.Params, string) ([]Post, int64, error)
}
