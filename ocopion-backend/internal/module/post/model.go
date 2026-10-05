package post

import (
	"time"

	"gorm.io/gorm"
)

// Post maps to the existing posts table.
type Post struct {
	ID        int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Title     string         `json:"title"`
	Content   string         `json:"content"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Post) TableName() string { return "posts" }

type ListResult struct {
	List     []Post `json:"list"`
	PageNum  int    `json:"pageNum"`
	PageSize int    `json:"pageSize"`
	Total    int64  `json:"total"`
}
