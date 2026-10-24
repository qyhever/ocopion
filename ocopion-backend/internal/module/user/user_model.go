package user

import (
	"time"

	"gorm.io/gorm"
)

// User maps to the existing users table. PasswordHash is never serialized.
type User struct {
	ID           int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Email        string         `json:"email" gorm:"size:128;not null;uniqueIndex:uk_users_email"`
	Phone        string         `json:"phone" gorm:"size:32;not null;default:''"`
	PasswordHash string         `json:"-" gorm:"size:255;not null"`
	Username     string         `json:"username" gorm:"size:128;not null"`
	Nickname     string         `json:"nickname" gorm:"size:128;not null"`
	AvatarURL    string         `json:"avatarUrl" gorm:"size:512;not null;default:''"`
	Status       string         `json:"status" gorm:"size:32;not null;default:active"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `json:"-"`
}

func (User) TableName() string { return "users" }

type CreateRequest struct {
	Email     string  `json:"email"`
	Phone     string  `json:"phone"`
	Password  string  `json:"password"`
	Username  string  `json:"username"`
	Nickname  string  `json:"nickname"`
	AvatarURL string  `json:"avatarUrl"`
	Status    *string `json:"status"`
}

type UpdateRequest struct {
	ID        int64   `json:"-"`
	Email     *string `json:"email"`
	Phone     *string `json:"phone"`
	Password  *string `json:"password"`
	Username  *string `json:"username"`
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatarUrl"`
	Status    *string `json:"status"`
}

type GetRequest struct {
	ID int64 `json:"-"`
}
type DeleteRequest struct {
	IDs []int64 `json:"ids"`
}
type DeleteResponse struct{}
type ListRequest struct {
	PageNum  int    `form:"pageNum"`
	PageSize int    `form:"pageSize"`
	Keyword  string `form:"keyword"`
}

type UserResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Username  string    `json:"username"`
	Nickname  string    `json:"nickname"`
	AvatarURL string    `json:"avatarUrl"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func newUserResponse(user User) UserResponse {
	return UserResponse{ID: user.ID, Email: user.Email, Phone: user.Phone, Username: user.Username,
		Nickname: user.Nickname, AvatarURL: user.AvatarURL, Status: user.Status, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
}

type ListResult struct {
	List     []User
	PageNum  int
	PageSize int
	Total    int64
}
type ListResponse struct {
	List     []UserResponse `json:"list"`
	PageNum  int            `json:"pageNum"`
	PageSize int            `json:"pageSize"`
	Total    int64          `json:"total"`
}

func newListResponse(result ListResult) ListResponse {
	list := make([]UserResponse, 0, len(result.List))
	for _, user := range result.List {
		list = append(list, newUserResponse(user))
	}
	return ListResponse{List: list, PageNum: result.PageNum, PageSize: result.PageSize, Total: result.Total}
}

// IsActive reports whether the user can authenticate.
func (u *User) IsActive() bool {
	return u != nil && u.ID > 0 && !u.DeletedAt.Valid && u.Status == "active"
}
