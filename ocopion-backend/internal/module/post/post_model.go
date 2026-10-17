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

type CreateRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type UpdateRequest struct {
	ID      int64   `json:"-"`
	Title   *string `json:"title"`
	Content *string `json:"content"`
}

type GetRequest struct {
	ID int64 `json:"-"`
}

type DeleteRequest struct {
	IDs []int64 `json:"ids"`
}

// DeleteResponse is nil because the delete endpoint returns null data.
type DeleteResponse struct{}

type ListRequest struct {
	PageNum  int    `form:"pageNum"`
	PageSize int    `form:"pageSize"`
	Keyword  string `form:"keyword"`
}

// PostResponse is the data returned by the create, update, and get endpoints.
type PostResponse struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func newPostResponse(post Post) PostResponse {
	return PostResponse{
		ID: post.ID, Title: post.Title, Content: post.Content,
		CreatedAt: post.CreatedAt, UpdatedAt: post.UpdatedAt,
	}
}

type ListResult struct {
	List     []Post `json:"list"`
	PageNum  int    `json:"pageNum"`
	PageSize int    `json:"pageSize"`
	Total    int64  `json:"total"`
}

type ListResponse struct {
	List     []PostResponse `json:"list"`
	PageNum  int            `json:"pageNum"`
	PageSize int            `json:"pageSize"`
	Total    int64          `json:"total"`
}

func newListResponse(result ListResult) ListResponse {
	list := make([]PostResponse, 0, len(result.List))
	for _, post := range result.List {
		list = append(list, newPostResponse(post))
	}
	return ListResponse{List: list, PageNum: result.PageNum, PageSize: result.PageSize, Total: result.Total}
}
