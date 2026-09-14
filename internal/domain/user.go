package domain

import (
	"time"
)

type User struct {
	ID        string    `json:"id" gorm:"type:char(36);primaryKey"`
	Username  string    `json:"username" gorm:"type:varchar(255);not null"`
	AvatarURL *string   `json:"avatar_url" gorm:"type:varchar(512)"`
	CreatedAt time.Time `json:"created_at" gorm:"type:timestamp;autoCreateTime"`
}

type CreateUserRequest struct {
	Username  string  `json:"username" binding:"required,min=2,max=50"`
	AvatarURL *string `json:"avatar_url,omitempty" binding:"omitempty,url"`
}

type UserResponse struct {
	User  *User  `json:"user"`
	Token string `json:"token,omitempty"`
}

type UserRepository interface {
	Create(user *User) error
	GetByID(id string) (*User, error)
	GetByUsername(username string) (*User, error)
}

type UserService interface {
	RegisterGuest(req *CreateUserRequest) (*UserResponse, error)
	GetProfile(userID string) (*User, error)
}
