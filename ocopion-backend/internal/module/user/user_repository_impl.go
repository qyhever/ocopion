package user

import (
	"context"
	"errors"

	"ocopion/internal/pkg/pagination"

	"github.com/go-sql-driver/mysql"

	"gorm.io/gorm"
)

type GormRepository struct{ db *gorm.DB }

func NewGormRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) Create(ctx context.Context, user *User) error {
	return translateError(r.db.WithContext(ctx).Create(user).Error)
}

func (r *GormRepository) Get(ctx context.Context, id int64) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *GormRepository) Update(ctx context.Context, id int64, fields map[string]interface{}) (*User, error) {
	result := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(fields)
	if result.Error != nil {
		return nil, translateError(result.Error)
	}
	return r.Get(ctx, id)
}

func (r *GormRepository) Delete(ctx context.Context, ids []int64) error {
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&User{}).Error
}

func (r *GormRepository) List(ctx context.Context, page pagination.Params, keyword string) ([]User, int64, error) {
	query := r.db.WithContext(ctx).Model(&User{})
	if keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("(email LIKE ? OR phone LIKE ? OR username LIKE ? OR nickname LIKE ?)", pattern, pattern, pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	users := make([]User, 0)
	if err := query.Order("id DESC").Limit(page.PageSize).Offset(page.Offset()).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

var _ Repository = (*GormRepository)(nil)

func translateError(err error) error {
	var mysqlErr *mysql.MySQLError
	if errors.Is(err, gorm.ErrDuplicatedKey) || errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return ErrEmailExists
	}
	return err
}

func (r *GormRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
