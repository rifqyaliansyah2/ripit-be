package service

import (
	"errors"
	"time"

	"ripit-be/internal/domain"
	"ripit-be/pkg/jwt"

	"github.com/google/uuid"
)

type userService struct {
	userRepo   domain.UserRepository
	tokenMaker *jwt.TokenMaker
}

func NewUserService(userRepo domain.UserRepository, tokenMaker *jwt.TokenMaker) domain.UserService {
	return &userService{
		userRepo:   userRepo,
		tokenMaker: tokenMaker,
	}
}

func (s *userService) RegisterGuest(req *domain.CreateUserRequest) (*domain.UserResponse, error) {
	if req.Username == "" {
		return nil, errors.New("username is required")
	}

	newUser := &domain.User{
		ID:        uuid.New().String(),
		Username:  req.Username,
		AvatarURL: req.AvatarURL,
		CreatedAt: time.Now(),
	}

	if err := s.userRepo.Create(newUser); err != nil {
		return nil, err
	}

	token, err := s.tokenMaker.GenerateToken(newUser.ID, newUser.Username)
	if err != nil {
		return nil, err
	}

	return &domain.UserResponse{
		User:  newUser,
		Token: token,
	}, nil
}

func (s *userService) GetProfile(userID string) (*domain.User, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}
	return user, nil
}
