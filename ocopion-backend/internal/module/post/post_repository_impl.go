package post

import (
	"context"

	"ocopion/internal/pkg/pagination"

	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Create(ctx context.Context, post *Post) error {
	return r.db.WithContext(ctx).Create(post).Error
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*Post, error) {
	var post Post
	if err := r.db.WithContext(ctx).First(&post, id).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]interface{}) (*Post, error) {
	result := r.db.WithContext(ctx).Model(&Post{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) Delete(ctx context.Context, ids []int64) error {
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&Post{}).Error
}

func (r *GormRepository) List(ctx context.Context, page pagination.Params, keyword string) ([]Post, int64, error) {
	query := r.db.WithContext(ctx).Model(&Post{})
	if keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR content LIKE ?", pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	posts := make([]Post, 0)
	if err := query.Order("id DESC").Limit(page.PageSize).Offset(page.Offset()).Find(&posts).Error; err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

var _ Repository = (*GormRepository)(nil)
