package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Log      LogConfig      `json:"log"`
}

type ServerConfig struct {
	Port int `json:"port"`
}

type DatabaseConfig struct {
	DSN string `json:"dsn"`
}

type LogConfig struct {
	Level string `json:"level"`
}

var GlobalConfig Config

func Init() {
	configFile := "config.json"
	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Printf("警告: 无法读取配置文件 %s: %v\n", configFile, err)
		GlobalConfig = Config{
			Server: ServerConfig{
				Port: 5020,
			},
			Database: DatabaseConfig{
				DSN: "root:Wxl111222@tcp(localhost:3306)/videos_website_db?charset=utf8mb4&parseTime=True&loc=Local",
			},
			Log: LogConfig{
				Level: "info",
			},
		}
	} else {
		err = json.Unmarshal(content, &GlobalConfig)
		if err != nil {
			log.Printf("警告: 解析配置文件失败: %v\n", err)
			GlobalConfig = Config{
				Server: ServerConfig{
					Port: 5020,
				},
				Database: DatabaseConfig{
					DSN: "root:Wxl111222@tcp(localhost:3306)/videos_website_db?charset=utf8mb4&parseTime=True&loc=Local",
				},
				Log: LogConfig{
					Level: "info",
				},
			}
		}
	}
}

func Get() *Config {
	return &GlobalConfig
}

func GetLogger() (*log.Logger, *os.File) {
	logFile, err := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("警告: 无法打开日志文件: %v\n", err)
		return log.New(os.Stdout, "", log.LstdFlags), nil
	}
	return log.New(os.Stdout, "", log.LstdFlags), logFile
}
