package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:100;uniqueIndex" json:"username"`
	Email     string    `gorm:"size:100;uniqueIndex" json:"email"`
	Password  string    `gorm:"size:100" json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type PlayHistory struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"index" json:"userId"`
	VideoID       string    `gorm:"size:255;index" json:"videoId"`
	AnimeTitle    string    `gorm:"size:255" json:"animeTitle"`
	Episode       string    `gorm:"size:255" json:"episode"`
	VideoURL      string    `gorm:"size:500" json:"videoUrl"`
	CurrentTime   float64   `json:"currentTime"`
	Duration      float64   `json:"duration"`
	Progress      float64   `json:"progress"`
	LastPlayed    time.Time `json:"lastPlayed"`
	SegmentID     string    `gorm:"size:100" json:"segmentId"`
	SegmentOffset float64   `json:"segmentOffset"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type AnimeInfo struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Title        string    `gorm:"size:255" json:"title"`
	Summary      string    `gorm:"size:1000" json:"summary"`
	Cover        string    `gorm:"size:255" json:"cover"`
	VideoURL     string    `gorm:"size:255" json:"video_url"`
	Episodes     int       `json:"episodes"`
	FolderName   string    `gorm:"size:255" json:"folder_name"`
	PhysicalPath string    `gorm:"size:500" json:"physical_path"`
	StorageDisk  string    `gorm:"size:100" json:"storage_disk"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type VideoFile struct {
	Path         string `json:"path"`
	FileName     string `json:"file_name"`
	PhysicalPath string `json:"physical_path"`
}

type BatchResult struct {
	Total   int      `json:"total"`
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &AnimeInfo{}, &PlayHistory{})
}
