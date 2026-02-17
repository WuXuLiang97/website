package services

import (
	"fmt"
	"log"
	"time"

	"anime-website/models"

	"gorm.io/gorm"
)

type PlayHistoryService struct{}

var PlayHistoryServiceInstance = &PlayHistoryService{}

func (s *PlayHistoryService) SavePlayHistory(userID uint, req *PlayHistoryRequest) error {
	if req.Duration > 0 {
		req.Progress = (req.CurrentTime / req.Duration) * 100
	} else {
		req.Progress = 0
	}

	if req.CurrentTime > req.Duration && req.Duration > 0 {
		req.CurrentTime = req.Duration
		req.Progress = 100
	}

	if !LocalMode && DB != nil {
		var history models.PlayHistory
		result := DB.Where("video_id = ? AND user_id = ?", req.VideoID, userID).First(&history)

		if result.Error == nil {
			history.AnimeTitle = req.AnimeTitle
			history.Episode = req.Episode
			history.VideoURL = req.VideoURL
			history.CurrentTime = req.CurrentTime
			history.Duration = req.Duration
			history.Progress = req.Progress
			history.LastPlayed = time.Now()
			history.SegmentID = req.SegmentID
			history.SegmentOffset = req.SegmentOffset
			history.UpdatedAt = time.Now()

			DB.Save(&history)
		} else if result.Error == gorm.ErrRecordNotFound {
			history = models.PlayHistory{
				UserID:        userID,
				VideoID:       req.VideoID,
				AnimeTitle:    req.AnimeTitle,
				Episode:       req.Episode,
				VideoURL:      req.VideoURL,
				CurrentTime:   req.CurrentTime,
				Duration:      req.Duration,
				Progress:      req.Progress,
				LastPlayed:    time.Now(),
				SegmentID:     req.SegmentID,
				SegmentOffset: req.SegmentOffset,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			DB.Create(&history)
		} else {
			log.Printf("错误: 查询播放记录失败: %v\n", result.Error)
		}
	} else {
		historyMu.Lock()
		defer historyMu.Unlock()

		historyKey := fmt.Sprintf("%d_%s", userID, req.VideoID)
		existingHistory, exists := historyMap[historyKey]
		if exists {
			existingHistory.AnimeTitle = req.AnimeTitle
			existingHistory.Episode = req.Episode
			existingHistory.VideoURL = req.VideoURL
			existingHistory.CurrentTime = req.CurrentTime
			existingHistory.Duration = req.Duration
			existingHistory.Progress = req.Progress
			existingHistory.LastPlayed = time.Now()
			existingHistory.SegmentID = req.SegmentID
			existingHistory.SegmentOffset = req.SegmentOffset
			existingHistory.UpdatedAt = time.Now()
			historyMap[historyKey] = existingHistory
		} else {
			historyMap[historyKey] = models.PlayHistory{
				UserID:        userID,
				VideoID:       req.VideoID,
				AnimeTitle:    req.AnimeTitle,
				Episode:       req.Episode,
				VideoURL:      req.VideoURL,
				CurrentTime:   req.CurrentTime,
				Duration:      req.Duration,
				Progress:      req.Progress,
				LastPlayed:    time.Now(),
				SegmentID:     req.SegmentID,
				SegmentOffset: req.SegmentOffset,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
		}
	}

	log.Printf("播放记录已保存: VideoID=%s, CurrentTime=%.2f, Progress=%.2f%%\n",
		req.VideoID, req.CurrentTime, req.Progress)

	return nil
}

func (s *PlayHistoryService) GetPlayHistory(userID uint, videoID string) (*models.PlayHistory, error) {
	var history models.PlayHistory

	if !LocalMode && DB != nil {
		result := DB.Where("video_id = ? AND user_id = ?", videoID, userID).First(&history)
		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				return nil, nil
			}
			log.Printf("错误: 查询播放记录失败: %v\n", result.Error)
			return nil, result.Error
		}
	} else {
		historyMu.Lock()
		historyKey := fmt.Sprintf("%d_%s", userID, videoID)
		existingHistory, exists := historyMap[historyKey]
		historyMu.Unlock()

		if !exists {
			return nil, nil
		}
		history = existingHistory
	}

	return &history, nil
}

func (s *PlayHistoryService) GetAllPlayHistory(userID uint) ([]models.PlayHistory, error) {
	var histories []models.PlayHistory

	if !LocalMode && DB != nil {
		result := DB.Where("user_id = ?", userID).Order("last_played DESC").Find(&histories)
		if result.Error != nil {
			log.Printf("错误: 获取所有播放记录失败: %v\n", result.Error)
			return nil, result.Error
		}
	} else {
		historyMu.Lock()
		defer historyMu.Unlock()

		for _, history := range historyMap {
			if history.UserID == userID {
				histories = append(histories, history)
			}
		}
	}

	return histories, nil
}

func (s *PlayHistoryService) DeletePlayHistory(userID uint, videoID string) error {
	if !LocalMode && DB != nil {
		result := DB.Where("video_id = ? AND user_id = ?", videoID, userID).Delete(&models.PlayHistory{})
		if result.Error != nil {
			log.Printf("错误: 删除播放记录失败: %v\n", result.Error)
			return result.Error
		}
	} else {
		historyMu.Lock()
		defer historyMu.Unlock()
		historyKey := fmt.Sprintf("%d_%s", userID, videoID)
		delete(historyMap, historyKey)
	}

	return nil
}

func (s *PlayHistoryService) ClearAllPlayHistory(userID uint) error {
	if !LocalMode && DB != nil {
		result := DB.Where("user_id = ?", userID).Delete(&models.PlayHistory{})
		if result.Error != nil {
			log.Printf("错误: 清除所有播放记录失败: %v\n", result.Error)
			return result.Error
		}
	} else {
		historyMu.Lock()
		defer historyMu.Unlock()

		for key, history := range historyMap {
			if history.UserID == userID {
				delete(historyMap, key)
			}
		}
	}

	return nil
}

type PlayHistoryRequest struct {
	VideoID       string  `json:"videoId"`
	AnimeTitle    string  `json:"animeTitle"`
	Episode       string  `json:"episode"`
	VideoURL      string  `json:"videoUrl"`
	CurrentTime   float64 `json:"currentTime"`
	Duration      float64 `json:"duration"`
	Progress      float64 `json:"progress"`
	SegmentID     string  `json:"segmentId"`
	SegmentOffset float64 `json:"segmentOffset"`
}
