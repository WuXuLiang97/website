package services

import (
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"anime-website/config"
	"anime-website/models"
)

var DB *gorm.DB
var LocalMode bool

func InitDB() {
	cfg := config.Get()
	var err error
	DB, err = gorm.Open(mysql.Open(cfg.Database.DSN), &gorm.Config{})
	if err != nil {
		log.Printf("错误: 无法连接到数据库: %v\n", err)
		LocalMode = true
		log.Println("警告: 启用本地模式，将使用文件系统而不是数据库")
		return
	}

	err = models.AutoMigrate(DB)
	if err != nil {
		log.Printf("错误: 数据库迁移失败: %v\n", err)
		LocalMode = true
		log.Println("警告: 启用本地模式，将使用文件系统而不是数据库")
		return
	}

	log.Println("数据库连接成功")
}

func GetDB() *gorm.DB {
	return DB
}
