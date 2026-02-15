package handlers

import (
	"log"
	"net/http"

	"anime-website/services"

	"github.com/gin-gonic/gin"
)

type PlayHistoryHandler struct {
	playHistoryService *services.PlayHistoryService
	authHandler        *AuthHandler
}

func NewPlayHistoryHandler(authHandler *AuthHandler) *PlayHistoryHandler {
	return &PlayHistoryHandler{
		playHistoryService: services.PlayHistoryServiceInstance,
		authHandler:        authHandler,
	}
}

func (h *PlayHistoryHandler) SavePlayHistory(c *gin.Context) {
	var req services.PlayHistoryRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("错误: 保存播放记录参数错误: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	if req.VideoID == "" {
		log.Println("错误: VideoID为空")
		c.JSON(http.StatusBadRequest, gin.H{"error": "VideoID不能为空"})
		return
	}

	userID, ok := h.authHandler.GetUserIDFromCookie(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	err := h.playHistoryService.SavePlayHistory(userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"currentTime": req.CurrentTime,
		"progress":    req.Progress,
	})
}

func (h *PlayHistoryHandler) GetPlayHistory(c *gin.Context) {
	videoId := c.Query("videoId")
	if videoId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少videoId"})
		return
	}

	userID, ok := h.authHandler.GetUserIDFromCookie(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	history, err := h.playHistoryService.GetPlayHistory(userID, videoId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	if history == nil {
		c.JSON(http.StatusOK, gin.H{
			"videoId":     videoId,
			"currentTime": 0,
			"duration":    0,
			"progress":    0,
			"hasRecord":   false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            history.ID,
		"videoId":       history.VideoID,
		"animeTitle":    history.AnimeTitle,
		"episode":       history.Episode,
		"videoUrl":      history.VideoURL,
		"currentTime":   history.CurrentTime,
		"duration":      history.Duration,
		"progress":      history.Progress,
		"lastPlayed":    history.LastPlayed.Format("2006-01-02 15:04:05"),
		"segmentId":     history.SegmentID,
		"segmentOffset": history.SegmentOffset,
		"hasRecord":     true,
	})
}

func (h *PlayHistoryHandler) GetAllPlayHistory(c *gin.Context) {
	userID, ok := h.authHandler.GetUserIDFromCookie(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	histories, err := h.playHistoryService.GetAllPlayHistory(userID)
	if err != nil {
		log.Printf("错误: 获取所有播放记录失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	type HistoryResponse struct {
		ID          uint    `json:"id"`
		VideoID     string  `json:"videoId"`
		AnimeTitle  string  `json:"animeTitle"`
		Episode     string  `json:"episode"`
		VideoURL    string  `json:"videoUrl"`
		CurrentTime float64 `json:"currentTime"`
		Duration    float64 `json:"duration"`
		Progress    float64 `json:"progress"`
		LastPlayed  string  `json:"lastPlayed"`
	}

	responses := make([]HistoryResponse, len(histories))
	for i, h := range histories {
		responses[i] = HistoryResponse{
			ID:          h.ID,
			VideoID:     h.VideoID,
			AnimeTitle:  h.AnimeTitle,
			Episode:     h.Episode,
			VideoURL:    h.VideoURL,
			CurrentTime: h.CurrentTime,
			Duration:    h.Duration,
			Progress:    h.Progress,
			LastPlayed:  h.LastPlayed.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"histories": responses,
		"total":     len(responses),
	})
}

func (h *PlayHistoryHandler) DeletePlayHistory(c *gin.Context) {
	videoId := c.Query("videoId")
	if videoId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少videoId"})
		return
	}

	userID, ok := h.authHandler.GetUserIDFromCookie(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	err := h.playHistoryService.DeletePlayHistory(userID, videoId)
	if err != nil {
		log.Printf("错误: 删除播放记录失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *PlayHistoryHandler) ClearAllPlayHistory(c *gin.Context) {
	userID, ok := h.authHandler.GetUserIDFromCookie(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	err := h.playHistoryService.ClearAllPlayHistory(userID)
	if err != nil {
		log.Printf("错误: 清除所有播放记录失败: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "清除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}
