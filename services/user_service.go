package services

import (
	"log"
	"time"

	"anime-website/models"
)

type UserService struct{}

var UserServiceInstance = &UserService{}

func (s *UserService) Register(username, email, password string) (*models.User, error) {
	var existingUser models.User
	result := DB.Where("username = ?", username).First(&existingUser)
	if result.Error == nil {
		return nil, &UserError{Message: "用户名已存在"}
	}

	result = DB.Where("email = ?", email).First(&existingUser)
	if result.Error == nil {
		return nil, &UserError{Message: "邮箱已存在"}
	}

	user := models.User{
		Username:  username,
		Email:     email,
		Password:  password,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result = DB.Create(&user)
	if result.Error != nil {
		log.Printf("错误: 创建用户失败: %v\n", result.Error)
		return nil, result.Error
	}

	return &user, nil
}

func (s *UserService) Login(username, password string) (*models.User, error) {
	var user models.User
	result := DB.Where("username = ?", username).First(&user)
	if result.Error != nil {
		return nil, &UserError{Message: "用户名或密码错误"}
	}

	if user.Password != password {
		return nil, &UserError{Message: "用户名或密码错误"}
	}

	return &user, nil
}

func (s *UserService) GetUserByID(id uint) (*models.User, error) {
	var user models.User
	result := DB.First(&user, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

type UserError struct {
	Message string
}

func (e *UserError) Error() string {
	return e.Message
}
