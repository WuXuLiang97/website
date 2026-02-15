package handlers

import (
	"encoding/json"
	"net/http"

	"anime-website/services"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userService *services.UserService
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userService: services.UserServiceInstance,
	}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "details": err.Error()})
		return
	}

	user, err := h.userService.Register(req.Username, req.Email, req.Password)
	if err != nil {
		if userErr, ok := err.(*services.UserError); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": userErr.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "注册失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "注册成功",
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
		},
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "details": err.Error()})
		return
	}

	user, err := h.userService.Login(req.Username, req.Password)
	if err != nil {
		if userErr, ok := err.(*services.UserError); ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": userErr.Message})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
		return
	}

	userInfo := gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	}

	userJSON, err := json.Marshal(userInfo)
	if err == nil {
		c.SetCookie(
			"user",
			string(userJSON),
			7*24*60*60,
			"/",
			"",
			false,
			true,
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "登录成功",
		"user":    userInfo,
	})
}

func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var userInfo gin.H
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"user":   userInfo,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie(
		"user",
		"",
		-1,
		"/",
		"",
		false,
		true,
	)

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "登出成功",
	})
}

func (h *AuthHandler) RenewCookie(c *gin.Context) {
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	user, err := h.userService.GetUserByID(uint(userID))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		return
	}

	newUserInfo := gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	}

	newUserJSON, err := json.Marshal(newUserInfo)
	if err == nil {
		c.SetCookie(
			"user",
			string(newUserJSON),
			7*24*60*60,
			"/",
			"",
			false,
			true,
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Cookie续签成功",
		"user":    newUserInfo,
	})
}

func (h *AuthHandler) GetUserIDFromCookie(c *gin.Context) (uint, bool) {
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		return 0, false
	}

	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		return 0, false
	}

	userID, ok := userInfo["id"].(float64)
	if !ok {
		return 0, false
	}

	return uint(userID), true
}
