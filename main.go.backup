package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path" // 新增：处理URL路径
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 配置结构体
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

// 用户结构体
type User struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"size:100;uniqueIndex" json:"username"` // 用户名
	Email     string    `gorm:"size:100;uniqueIndex" json:"email"`    // 邮箱
	Password  string    `gorm:"size:100" json:"-"`                    // 密码（不返回给前端）
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 播放记录结构体
type PlayHistory struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"index" json:"userId"`           // 用户ID，关联到User表
	VideoID       string    `gorm:"size:255;index" json:"videoId"` // 视频唯一标识（如视频文件路径）
	AnimeTitle    string    `gorm:"size:255" json:"animeTitle"`    // 动画标题
	Episode       string    `gorm:"size:255" json:"episode"`       // 集数名称
	VideoURL      string    `gorm:"size:500" json:"videoUrl"`      // 视频URL
	CurrentTime   float64   `json:"currentTime"`                   // 全局播放位置（秒）
	Duration      float64   `json:"duration"`                      // 视频总时长（秒）
	Progress      float64   `json:"progress"`                      // 播放进度百分比
	LastPlayed    time.Time `json:"lastPlayed"`                    // 最后播放时间
	SegmentID     string    `gorm:"size:100" json:"segmentId"`     // 当前HLS切片ID（可选，用于调试）
	SegmentOffset float64   `json:"segmentOffset"`                 // 在当前切片内的偏移量（秒）
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// 内存存储（仅在本地模式或数据库不可用时使用）
var (
	historyMap = make(map[string]PlayHistory)
	historyMu  sync.Mutex
)

// 动画信息结构体
type AnimeInfo struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Title      string    `gorm:"size:255" json:"title"`
	Summary    string    `gorm:"size:1000" json:"summary"`
	Cover      string    `gorm:"size:255" json:"cover"`
	VideoURL   string    `gorm:"size:255" json:"video_url"`
	Episodes   int       `json:"episodes"`
	FolderName string    `gorm:"size:255" json:"folder_name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// 视频文件结构体
type VideoFile struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
}

// 批量处理结果
type BatchResult struct {
	Total   int      `json:"total"`
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"`
}

// 全局变量
var (
	config         Config
	db             *gorm.DB
	localMode      bool
	videosDir      = "static/videos"
	hlsDir         = "static/hls"
	templatesDir   = "templates"
	allowedFormats = []string{".mp4", ".flv", ".mkv", ".avi"}
	maxWorkers     = 5
	logFile        *os.File
	logger         *log.Logger
)

// 新增：标准化URL路径（关键修复）
//确保路径使用正斜杠，无重复斜杠，无反斜杠

func normalizeURLPath(p string) string {
	// 替换所有反斜杠为正斜杠
	p = strings.ReplaceAll(p, "\\", "/")
	// 拆分路径并重新拼接，去除重复斜杠
	parts := strings.FieldsFunc(p, func(r rune) bool {
		return r == '/'
	})
	if len(parts) == 0 {
		return "/"
	}
	// 确保路径以/开头
	return "/" + strings.Join(parts, "/")
}

// 初始化配置
func initConfig() {
	configFile := "config.json"
	content, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Printf("警告: 无法读取配置文件 %s: %v\n", configFile, err)
		config = Config{
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
		err = json.Unmarshal(content, &config)
		if err != nil {
			log.Printf("警告: 解析配置文件失败: %v\n", err)
			config = Config{
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

// 初始化日志
func initLogger() {
	var err error
	logFile, err = os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("警告: 无法打开日志文件: %v\n", err)
		logger = log.New(os.Stdout, "", log.LstdFlags)
	} else {
		logger = log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags)
	}
}

// 初始化数据库
func initDB() {
	var err error
	db, err = gorm.Open(mysql.Open(config.Database.DSN), &gorm.Config{})
	if err != nil {
		logger.Printf("错误: 无法连接到数据库: %v\n", err)
		localMode = true
		logger.Println("警告: 启用本地模式，将使用文件系统而不是数据库")
		return
	}

	// 自动迁移表结构
	err = db.AutoMigrate(&User{}, &AnimeInfo{}, &PlayHistory{})
	if err != nil {
		logger.Printf("错误: 数据库迁移失败: %v\n", err)
		localMode = true
		logger.Println("警告: 启用本地模式，将使用文件系统而不是数据库")
		return
	}

	logger.Println("数据库连接成功")
}

// 检查文件是否是视频文件
func isVideoFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	for _, format := range allowedFormats {
		if ext == format {
			return true
		}
	}
	return false
}

// 扫描本地视频文件
func scanVideos() []AnimeInfo {
	var animes []AnimeInfo
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// 确保HLS目录存在
	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		logger.Printf("警告: HLS目录 %s 不存在，将创建\n", hlsDir)
		err = os.MkdirAll(hlsDir, 0755)
		if err != nil {
			logger.Printf("错误: 创建HLS目录失败: %v\n", err)
			return animes
		}
	}

	// 扫描HLS目录
	entries, err := ioutil.ReadDir(hlsDir)
	if err != nil {
		logger.Printf("错误: 扫描HLS目录失败: %v\n", err)
		return animes
	}

	// 遍历子目录（每个子目录代表一个动画）
	for _, entry := range entries {
		if entry.IsDir() {
			animeName := entry.Name()
			hlsAnimePath := filepath.Join(hlsDir, animeName)

			wg.Add(1)
			go func(name string, hlsFolder string) {
				defer wg.Done()

				var videos []VideoFile
				// 扫描HLS目录中的子目录（每个子目录代表一集）
				hlsEntries, err := ioutil.ReadDir(hlsFolder)
				if err == nil {
					for _, hlsEntry := range hlsEntries {
						if hlsEntry.IsDir() {
							// 检查是否存在playlist.m3u8文件
							playlistPath := filepath.Join(hlsFolder, hlsEntry.Name(), "playlist.m3u8")
							if _, err := os.Stat(playlistPath); err == nil {
								// 构建HLS URL路径
								hlsURL := normalizeURLPath(path.Join("/hls", name, hlsEntry.Name(), "playlist.m3u8"))
								videos = append(videos, VideoFile{
									Path:     hlsURL,
									FileName: hlsEntry.Name(),
								})
							}
						}
					}
				}

				// 如果有视频文件，创建动画信息
				if len(videos) > 0 {
					// 按文件名排序
					sort.Slice(videos, func(i, j int) bool {
						return videos[i].FileName < videos[j].FileName
					})

					// 使用第一个视频文件作为主视频
					mainVideo := videos[0]

					// 检查封面文件
					coverURL := "/static/css/default-cover.jpg" // 默认封面
					coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

					for _, format := range coverFormats {
						// 检查HLS目录中的封面文件
						hlsCoverPath := filepath.Join(hlsDir, name, format)
						if _, err := os.Stat(hlsCoverPath); err == nil {
							// 修复：标准化封面URL路径
							coverURL = normalizeURLPath(path.Join("/hls", name, format))
							break
						}
					}

					anime := AnimeInfo{
						Title:      name,
						Summary:    fmt.Sprintf("这是一部名为 %s 的动画", name),
						Cover:      coverURL,
						VideoURL:   mainVideo.Path,
						Episodes:   len(videos),
						FolderName: name,
					}

					mutex.Lock()
					animes = append(animes, anime)
					mutex.Unlock()

					// 更新或插入到数据库
					if !localMode {
						updateAnimeInfo(anime)
					}
				}
			}(animeName, hlsAnimePath)
		}
	}

	wg.Wait()

	// 按标题排序
	sort.Slice(animes, func(i, j int) bool {
		return animes[i].Title < animes[j].Title
	})

	return animes
}

// 更新动画信息到数据库
func updateAnimeInfo(anime AnimeInfo) {
	var existingAnime AnimeInfo
	result := db.Where("folder_name = ?", anime.FolderName).First(&existingAnime)

	if result.Error == nil {
		// 更新现有记录
		existingAnime.Title = anime.Title
		existingAnime.Summary = anime.Summary
		existingAnime.Cover = anime.Cover
		existingAnime.VideoURL = anime.VideoURL
		existingAnime.Episodes = anime.Episodes
		existingAnime.UpdatedAt = time.Now()

		result = db.Save(&existingAnime)
		if result.Error != nil {
			logger.Printf("错误: 更新动画信息失败: %v\n", result.Error)
		}
	} else if result.Error == gorm.ErrRecordNotFound {
		// 创建新记录
		anime.CreatedAt = time.Now()
		anime.UpdatedAt = time.Now()

		result = db.Create(&anime)
		if result.Error != nil {
			logger.Printf("错误: 创建动画信息失败: %v\n", result.Error)
		}
	} else {
		logger.Printf("错误: 查询动画信息失败: %v\n", result.Error)
	}
}

// 从数据库获取动画信息
func getAnimesFromDB() []AnimeInfo {
	var animes []AnimeInfo
	result := db.Find(&animes)
	if result.Error != nil {
		logger.Printf("错误: 从数据库获取动画信息失败: %v\n", result.Error)
		return scanVideos() // 回退到扫描本地文件
	}

	// 检查并更新封面路径
	for i := range animes {
		// 检查当前封面路径是否存在
		currentCoverPath := strings.TrimPrefix(animes[i].Cover, "/")
		if _, err := os.Stat(currentCoverPath); os.IsNotExist(err) {
			// 封面文件不存在，重新生成封面路径
			coverURL := "/static/css/default-cover.jpg" // 默认封面
			coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

			for _, format := range coverFormats {
				// 检查HLS目录中的封面文件
				hlsCoverPath := filepath.Join(hlsDir, animes[i].FolderName, format)
				if _, err := os.Stat(hlsCoverPath); err == nil {
					coverURL = normalizeURLPath(path.Join("/hls", animes[i].FolderName, format))
					break
				}
				// 检查原视频目录中的封面文件
				coverPath := filepath.Join(videosDir, animes[i].FolderName, format)
				if _, err := os.Stat(coverPath); err == nil {
					coverURL = normalizeURLPath(path.Join("/", videosDir, animes[i].FolderName, format))
					break
				}
			}

			// 更新封面路径
			animes[i].Cover = coverURL
			// 同时更新数据库
			db.Model(&animes[i]).Update("cover", coverURL)
		}
	}

	return animes
}

// 搜索动画
func searchAnimes(keyword string) []AnimeInfo {
	var animes []AnimeInfo

	if !localMode {
		result := db.Where("title LIKE ? OR folder_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%").Find(&animes)
		if result.Error != nil {
			logger.Printf("错误: 搜索动画失败: %v\n", result.Error)
			// 回退到本地搜索
			animes = scanVideos()
		} else {
			return animes
		}
	}

	// 本地搜索
	allAnimes := scanVideos()
	for _, anime := range allAnimes {
		if strings.Contains(strings.ToLower(anime.Title), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(anime.FolderName), strings.ToLower(keyword)) {
			animes = append(animes, anime)
		}
	}

	return animes
}

// 获取动画信息
func getAnimeInfo(folderName string) (AnimeInfo, bool) {
	var anime AnimeInfo

	if !localMode {
		result := db.Where("folder_name = ?", folderName).First(&anime)
		if result.Error == nil {
			return anime, true
		} else if result.Error != gorm.ErrRecordNotFound {
			logger.Printf("错误: 获取动画信息失败: %v\n", result.Error)
		}
	}

	// 1. 检查原始视频目录
	animeFolder := filepath.Join(videosDir, folderName)
	var videos []VideoFile
	var hasVideoFiles bool

	// 扫描原始视频目录中的视频文件
	if _, err := os.Stat(animeFolder); err == nil {
		videoFiles, err := ioutil.ReadDir(animeFolder)
		if err == nil {
			for _, file := range videoFiles {
				if !file.IsDir() && isVideoFile(file.Name()) {
					videoPath := normalizeURLPath(path.Join("/", videosDir, folderName, file.Name()))
					videos = append(videos, VideoFile{
						Path:     videoPath,
						FileName: file.Name(),
					})
					hasVideoFiles = true
				}
			}
		}
	}

	// 2. 如果原始视频目录不存在或没有视频文件，检查HLS目录
	hlsFolder := filepath.Join(hlsDir, folderName)
	if !hasVideoFiles {
		if _, err := os.Stat(hlsFolder); err == nil {
			hlsEntries, err := ioutil.ReadDir(hlsFolder)
			if err == nil {
				for _, entry := range hlsEntries {
					if entry.IsDir() {
						// 检查是否存在playlist.m3u8文件
						playlistPath := filepath.Join(hlsFolder, entry.Name(), "playlist.m3u8")
						if _, err := os.Stat(playlistPath); err == nil {
							// 构建HLS URL路径
							hlsURL := normalizeURLPath(path.Join("/hls", folderName, entry.Name(), "playlist.m3u8"))
							videos = append(videos, VideoFile{
								Path:     hlsURL,
								FileName: entry.Name(),
							})
							hasVideoFiles = true
						}
					}
				}
			}
		}
	}

	// 如果找到视频文件（无论是原始视频还是HLS），创建动画信息
	if hasVideoFiles && len(videos) > 0 {
		// 按文件名排序
		sort.Slice(videos, func(i, j int) bool {
			return videos[i].FileName < videos[j].FileName
		})

		// 使用第一个视频文件作为主视频
		mainVideo := videos[0]

		// 检查封面文件
		coverURL := "/static/css/default-cover.jpg" // 默认封面
		coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

		for _, format := range coverFormats {
			// 先检查HLS目录中的封面文件
			hlsCoverPath := filepath.Join(hlsDir, folderName, format)
			if _, err := os.Stat(hlsCoverPath); err == nil {
				coverURL = normalizeURLPath(path.Join("/hls", folderName, format))
				break
			}
			// 再检查原视频目录中的封面文件
			coverPath := filepath.Join(animeFolder, format)
			if _, err := os.Stat(coverPath); err == nil {
				coverURL = normalizeURLPath(path.Join("/", videosDir, folderName, format))
				break
			}
		}

		anime = AnimeInfo{
			Title:      folderName,
			Summary:    fmt.Sprintf("这是一部名为 %s 的动画", folderName),
			Cover:      coverURL,
			VideoURL:   mainVideo.Path,
			Episodes:   len(videos),
			FolderName: folderName,
		}

		// 检查是否有HLS切片
		if !strings.Contains(mainVideo.Path, "/hls/") {
			hlsPath := getHLSURL(mainVideo.Path)
			if _, err := os.Stat(strings.TrimPrefix(hlsPath, "/")); err == nil {
				anime.VideoURL = hlsPath
			}
		}

		return anime, true
	}

	return anime, false
}

// 获取动画的所有视频文件
func getAnimeVideos(folderName string) []VideoFile {
	var videos []VideoFile
	addedVideos := make(map[string]bool)

	animeFolder := filepath.Join(videosDir, folderName)

	// 1. 扫描原始视频目录
	if _, err := os.Stat(animeFolder); err == nil {
		videoFiles, err := ioutil.ReadDir(animeFolder)
		if err == nil {
			for _, file := range videoFiles {
				if !file.IsDir() && isVideoFile(file.Name()) {
					videoPath := normalizeURLPath(path.Join("/", videosDir, folderName, file.Name()))

					hlsPath := getHLSURL(videoPath)
					if _, err := os.Stat(strings.TrimPrefix(hlsPath, "/")); err == nil {
						videoPath = hlsPath
					}

					if !addedVideos[videoPath] {
						videos = append(videos, VideoFile{
							Path:     videoPath,
							FileName: file.Name(),
						})
						addedVideos[videoPath] = true
					}
				}
			}
		}
	}

	// 2. 扫描HLS目录，查找该动画的HLS切片
	hlsAnimePath := filepath.Join(hlsDir, folderName)
	if _, err := os.Stat(hlsAnimePath); err == nil {
		hlsEntries, err := ioutil.ReadDir(hlsAnimePath)
		if err == nil {
			for _, entry := range hlsEntries {
				if entry.IsDir() {
					// 检查是否存在playlist.m3u8文件
					playlistPath := filepath.Join(hlsAnimePath, entry.Name(), "playlist.m3u8")
					if _, err := os.Stat(playlistPath); err == nil {
						// 构建HLS URL路径
						hlsURL := normalizeURLPath(path.Join("/hls", folderName, entry.Name(), "playlist.m3u8"))

						// HLS视频使用实际目录名（不含扩展名），更符合实际播放格式
						hlsFileName := entry.Name()

						// 避免重复添加
						if !addedVideos[hlsURL] {
							videos = append(videos, VideoFile{
								Path:     hlsURL,
								FileName: hlsFileName,
							})
							addedVideos[hlsURL] = true
						}
					}
				}
			}
		}
	}

	// 按文件名排序
	sort.Slice(videos, func(i, j int) bool {
		return videos[i].FileName < videos[j].FileName
	})

	return videos
}

// 获取HLS目录（文件系统路径）
func getHLSDir(videoPath string) string {
	// 修复：先标准化URL路径，再转换为文件系统路径
	normalizedPath := normalizeURLPath(videoPath)
	// 移除URL前缀 /static/videos/
	relativePath := strings.TrimPrefix(normalizedPath, "/static/videos/")
	// 移除文件扩展名
	baseName := strings.TrimSuffix(relativePath, filepath.Ext(relativePath))
	// 构建HLS目录路径（文件系统路径，用filepath.Join）
	hlsDirPath := filepath.Join(hlsDir, baseName)
	return hlsDirPath
}

// 获取HLS URL（URL路径）
func getHLSURL(videoPath string) string {
	// 修复：先标准化URL路径
	normalizedPath := normalizeURLPath(videoPath)
	// 移除URL前缀 /static/videos/
	relativePath := strings.TrimPrefix(normalizedPath, "/static/videos/")
	// 移除文件扩展名
	baseName := strings.TrimSuffix(relativePath, path.Ext(relativePath)) // 使用path.Ext处理URL扩展名
	// 构建HLS URL（用path.Join确保正斜杠）
	hlsURL := normalizeURLPath(path.Join("/hls", baseName, "playlist.m3u8"))
	return hlsURL
}

// 已移动封面的目录记录
var movedCoverDirs = make(map[string]bool)
var movedCoverDirsMutex sync.Mutex

// 从视频路径中提取动画目录名称
func extractAnimeDirectory(videoPath string) string {
	// 标准化路径
	normalizedPath := normalizeURLPath(videoPath)

	// 移除URL前缀
	relativePath := strings.TrimPrefix(normalizedPath, "/static/")

	// 拆分路径
	pathParts := strings.Split(relativePath, "/")

	// 检查路径格式
	if len(pathParts) >= 2 {
		// 对于 /static/videos/动画名称/视频文件.mp4 格式
		if pathParts[0] == "videos" {
			return pathParts[1]
		}
		// 对于 /static/hls/动画名称/集数/playlist.m3u8 格式
		if pathParts[0] == "hls" && len(pathParts) >= 2 {
			return pathParts[1]
		}
	}

	return ""
}

// 增量扫描指定的动画目录
func scanAnimeDirectories(directories []string) []AnimeInfo {
	var animes []AnimeInfo
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// 去重目录列表
	dirMap := make(map[string]bool)
	uniqueDirs := []string{}
	for _, dir := range directories {
		if dir != "" && !dirMap[dir] {
			dirMap[dir] = true
			uniqueDirs = append(uniqueDirs, dir)
		}
	}

	logger.Printf("开始增量扫描 %d 个动画目录...", len(uniqueDirs))

	// 遍历每个目录
	for _, dirName := range uniqueDirs {
		hlsAnimePath := filepath.Join(hlsDir, dirName)

		// 检查目录是否存在
		if _, err := os.Stat(hlsAnimePath); os.IsNotExist(err) {
			logger.Printf("警告: 目录 %s 不存在，跳过扫描", hlsAnimePath)
			continue
		}

		wg.Add(1)
		go func(name string, hlsFolder string) {
			defer wg.Done()

			var videos []VideoFile
			// 扫描HLS目录中的子目录（每个子目录代表一集）
			hlsEntries, err := ioutil.ReadDir(hlsFolder)
			if err == nil {
				for _, hlsEntry := range hlsEntries {
					if hlsEntry.IsDir() {
						// 检查是否存在playlist.m3u8文件
						playlistPath := filepath.Join(hlsFolder, hlsEntry.Name(), "playlist.m3u8")
						if _, err := os.Stat(playlistPath); err == nil {
							// 构建HLS URL路径
							hlsURL := normalizeURLPath(path.Join("/hls", name, hlsEntry.Name(), "playlist.m3u8"))
							videos = append(videos, VideoFile{
								Path:     hlsURL,
								FileName: hlsEntry.Name(),
							})
						}
					}
				}
			}

			// 如果有视频文件，创建动画信息
			if len(videos) > 0 {
				// 按文件名排序
				sort.Slice(videos, func(i, j int) bool {
					return videos[i].FileName < videos[j].FileName
				})

				// 使用第一个视频文件作为主视频
				mainVideo := videos[0]

				// 检查封面文件
				coverURL := "/static/css/default-cover.jpg" // 默认封面
				coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

				for _, format := range coverFormats {
					// 检查HLS目录中的封面文件
					hlsCoverPath := filepath.Join(hlsDir, name, format)
					if _, err := os.Stat(hlsCoverPath); err == nil {
						// 标准化封面URL路径
						coverURL = normalizeURLPath(path.Join("/hls", name, format))
						break
					}
				}

				anime := AnimeInfo{
					Title:      name,
					Summary:    fmt.Sprintf("这是一部名为 %s 的动画", name),
					Cover:      coverURL,
					VideoURL:   mainVideo.Path,
					Episodes:   len(videos),
					FolderName: name,
				}

				mutex.Lock()
				animes = append(animes, anime)
				mutex.Unlock()

				// 更新或插入到数据库
				if !localMode {
					updateAnimeInfo(anime)
				}
			}
		}(dirName, hlsAnimePath)
	}

	wg.Wait()

	// 按标题排序
	sort.Slice(animes, func(i, j int) bool {
		return animes[i].Title < animes[j].Title
	})

	logger.Printf("增量扫描完成，更新了 %d 个动画信息", len(animes))

	return animes
}

// 移动封面文件到HLS目录
func moveCoverToHLS(videoPath string) {
	// 修复：先标准化URL路径
	normalizedPath := normalizeURLPath(videoPath)
	// 移除URL前缀 /static/videos/
	relativePath := strings.TrimPrefix(normalizedPath, "/static/videos/")
	// 提取动画标题（从路径中提取）
	pathParts := strings.Split(relativePath, "/")
	if len(pathParts) < 1 {
		logger.Printf("警告: 无法从视频路径中提取动画标题: %s\n", videoPath)
		return
	}

	animeTitle := pathParts[0]

	// 检查是否已经移动过该目录的封面
	movedCoverDirsMutex.Lock()
	if movedCoverDirs[animeTitle] {
		movedCoverDirsMutex.Unlock()
		return
	}
	movedCoverDirs[animeTitle] = true
	movedCoverDirsMutex.Unlock()

	// 构建视频目录路径
	videoDir := filepath.Join(videosDir, animeTitle)

	// 检查视频目录是否存在
	if _, err := os.Stat(videoDir); os.IsNotExist(err) {
		logger.Printf("警告: 视频目录 %s 不存在，无法移动封面文件\n", videoDir)
		return
	}

	// 构建HLS目录路径
	hlsDirPath := filepath.Join(hlsDir, animeTitle)

	// 确保HLS目录存在
	if err := os.MkdirAll(hlsDirPath, 0755); err != nil {
		logger.Printf("警告: 创建HLS目录失败: %v\n", err)
		return
	}

	// 检查封面文件
	coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}
	for _, format := range coverFormats {
		coverPath := filepath.Join(videoDir, format)
		// 再次检查文件是否存在（防止并发操作导致文件已被移动）
		if _, err := os.Stat(coverPath); err == nil {
			// 封面文件存在，移动到HLS目录
			targetPath := filepath.Join(hlsDirPath, format)
			if err := os.Rename(coverPath, targetPath); err != nil {
				// 检查是否是跨磁盘移动失败
				logger.Printf("警告: 移动封面文件失败: %v，尝试复制方式\n", err)
				// 使用复制+删除的方式实现跨磁盘移动
				if data, readErr := ioutil.ReadFile(coverPath); readErr == nil {
					if writeErr := ioutil.WriteFile(targetPath, data, 0644); writeErr == nil {
						// 复制成功后删除原文件
						if deleteErr := os.Remove(coverPath); deleteErr == nil {
							logger.Printf("成功: 封面文件已复制到HLS目录: %s\n", targetPath)
						} else {
							logger.Printf("警告: 复制封面文件成功，但删除原文件失败: %v\n", deleteErr)
						}
					} else {
						logger.Printf("警告: 复制封面文件失败: %v\n", writeErr)
					}
				} else {
					logger.Printf("警告: 读取封面文件失败: %v\n", readErr)
				}
			} else {
				logger.Printf("成功: 封面文件已移动到HLS目录: %s\n", targetPath)
			}
			break
		}
	}
}

// 生成HLS切片（无转码，仅切片）
func generateHLS(videoPath string) error {
	// 构建HLS目录路径
	hlsDirPath := getHLSDir(videoPath)

	// 确保HLS目录存在
	err := os.MkdirAll(hlsDirPath, 0755)
	if err != nil {
		return fmt.Errorf("创建HLS目录失败: %v", err)
	}

	// 构建输出文件路径
	playlistPath := filepath.Join(hlsDirPath, "playlist.m3u8")
	segmentPath := filepath.Join(hlsDirPath, "segment_%03d.ts")

	// 修复：转换URL路径为文件系统路径（处理Windows）
	videoFilePath := strings.TrimPrefix(normalizeURLPath(videoPath), "/")
	// Windows下需要将/转换为\
	videoFilePath = filepath.FromSlash(videoFilePath)

	// 构建FFmpeg命令（优化版：源校准+规范切片+音视频对齐+批量处理优化）
	// 构建FFmpeg命令（无转码仅切片，适配2025最新版FFmpeg，修复-sync参数报错）
	cmd := exec.Command(
		"ffmpeg",
		"-err_detect", "ignore_err", // 忽略源视频轻微格式错误
		"-i", videoFilePath, // 输入文件
		"-c:v", "copy", // 视频流复制，不转码
		"-c:a", "copy", // 音频流复制，不转码
		"-hls_time", "8", // 切片时长8s
		"-hls_list_size", "0", // 保留所有切片
		"-hls_segment_filename", segmentPath, // 切片路径模板
		// 核心修复：移除弃用的split_on_keyframe，保留其他有效flag
		"-hls_flags", "discont_start+temp_file+independent_segments",
		"-avoid_negative_ts", "make_zero", // 时间戳从0开始
		"-fflags", "+genpts+igndts", // 重生成时间戳，忽略DTS错误
		"-reset_timestamps", "1", // 每个切片重置时间戳
		"-loglevel", "error", // 仅输出错误日志
		playlistPath, // 输出m3u8
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行FFmpeg命令失败: %v, 输出: %s", err, string(output))
	}

	// 删除原始.mp4文件
	if strings.HasSuffix(strings.ToLower(videoFilePath), ".mp4") {
		err = os.Remove(videoFilePath)
		if err != nil {
			logger.Printf("警告: 删除原始文件失败: %v\n", err)
			// 不返回错误，继续执行
		} else {
			logger.Printf("成功删除原始文件: %s\n", videoFilePath)
		}
	}

	return nil
}

// 生成HLS切片（使用GPU加速，无转码）
func generateHLSHighQuality(videoPath string) error {
	// 构建HLS目录路径
	hlsDirPath := getHLSDir(videoPath)

	// 确保HLS目录存在
	err := os.MkdirAll(hlsDirPath, 0755)
	if err != nil {
		return fmt.Errorf("创建HLS目录失败: %v", err)
	}

	// 构建输出文件路径
	playlistPath := filepath.Join(hlsDirPath, "playlist.m3u8")
	segmentPath := filepath.Join(hlsDirPath, "segment_%03d.ts")

	// 修复：转换URL路径为文件系统路径
	videoFilePath := strings.TrimPrefix(normalizeURLPath(videoPath), "/")
	videoFilePath = filepath.FromSlash(videoFilePath)

	// 构建FFmpeg命令（使用GPU加速，无转码）
	// 构建FFmpeg命令（优化版：源校准+规范切片+音视频对齐+批量处理优化）
	// 构建FFmpeg命令（无转码仅切片，适配2025最新版FFmpeg，修复-sync参数报错）
	cmd := exec.Command(
		"ffmpeg",
		"-err_detect", "ignore_err", // 忽略源视频轻微格式错误
		"-i", videoFilePath, // 输入文件
		"-c:v", "copy", // 视频流复制，不转码
		"-c:a", "copy", // 音频流复制，不转码
		"-hls_time", "8", // 切片时长8s
		"-hls_list_size", "0", // 保留所有切片
		"-hls_segment_filename", segmentPath, // 切片路径模板
		// 核心修复：移除弃用的split_on_keyframe，保留其他有效flag
		"-hls_flags", "discont_start+temp_file+independent_segments",
		"-avoid_negative_ts", "make_zero", // 时间戳从0开始
		"-fflags", "+genpts+igndts", // 重生成时间戳，忽略DTS错误
		"-reset_timestamps", "1", // 每个切片重置时间戳
		"-loglevel", "error", // 仅输出错误日志
		playlistPath, // 输出m3u8
	)

	// 执行命令
	_, err = cmd.CombinedOutput()
	if err != nil {
		// 如果GPU加速失败，尝试使用CPU
		logger.Printf("警告: GPU加速失败，尝试使用CPU: %v\n", err)
		return generateHLS(videoPath)
	}

	// 删除原始.mp4文件
	// 使用已定义的videoFilePath变量，不需要重新定义

	if strings.HasSuffix(strings.ToLower(videoFilePath), ".mp4") {
		err = os.Remove(videoFilePath)
		if err != nil {
			logger.Printf("警告: 删除原始文件失败: %v\n", err)
			// 不返回错误，继续执行
		} else {
			logger.Printf("成功删除原始文件: %s\n", videoFilePath)
		}
	}

	return nil
}

// 全局变量：用于存储当前的批量处理上下文
var batchProcessingContexts = make(map[string]map[string]interface{})
var batchProcessingMutex sync.Mutex

// 批量生成HLS切片
func batchGenerateHLS(videos []string, useGPU bool, progressChan chan<- map[string]interface{}, stopChan <-chan struct{}) (int, int, int, int, []string) {
	total := len(videos)
	success := 0
	failed := 0
	skipped := 0 // 新增：记录跳过的视频数
	var errors []string

	// 使用信号量控制并发
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, videoPath := range videos {
		// 检查是否需要停止
		select {
		case <-stopChan:
			// 发送停止消息（使用非阻塞发送，防止通道已关闭）
			select {
			case progressChan <- map[string]interface{}{
				"type":      "stop",
				"message":   "处理已停止",
				"timestamp": time.Now().Format(time.RFC3339),
			}:
				// 消息发送成功
			default:
				// 通道已关闭，忽略
			}
			// 等待已开始的任务完成
			wg.Wait()
			return total, success, failed, skipped, errors
		default:
			// 继续处理
		}

		// 修复：标准化视频路径
		normalizedPath := normalizeURLPath(videoPath)

		// 检查HLS切片是否已存在
		hlsPath := getHLSURL(normalizedPath)
		hlsFilePath := strings.TrimPrefix(hlsPath, "/")

		if _, err := os.Stat(hlsFilePath); err == nil {
			// HLS切片已存在，跳过处理
			skipped++
			// 发送跳过消息（使用非阻塞发送，防止通道已关闭）
			select {
			case progressChan <- map[string]interface{}{
				"type":      "skipped",
				"video":     normalizedPath,
				"message":   "HLS切片已存在，跳过处理",
				"timestamp": time.Now().Format(time.RFC3339),
			}:
				// 消息发送成功
			default:
				// 通道已关闭，忽略
			}
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(idx int, path string) {
			defer func() {
				wg.Done()
				<-semaphore // 释放信号量
			}()

			// 发送进度更新（使用非阻塞发送，防止通道已关闭）
			select {
			case progressChan <- map[string]interface{}{
				"type":      "progress",
				"current":   idx + 1,
				"total":     total,
				"video":     path,
				"status":    "processing",
				"timestamp": time.Now().Format(time.RFC3339),
			}:
				// 消息发送成功
			default:
				// 通道已关闭，忽略
			}

			// 生成HLS切片
			var err error
			if useGPU {
				err = generateHLSHighQuality(path)
			} else {
				err = generateHLS(path)
			}

			if err != nil {
				failed++
				errorMsg := fmt.Sprintf("视频 %s 生成失败: %v", path, err)
				errors = append(errors, errorMsg)
				logger.Printf("错误: %s\n", errorMsg)

				// 发送错误更新（使用非阻塞发送，防止通道已关闭）
				select {
				case progressChan <- map[string]interface{}{
					"type":      "error",
					"video":     path,
					"message":   err.Error(),
					"timestamp": time.Now().Format(time.RFC3339),
				}:
					// 消息发送成功
				default:
					// 通道已关闭，忽略
				}
			} else {
				success++
				logger.Printf("成功: 视频 %s 生成HLS切片完成\n", path)

				// 移动封面文件到HLS目录
				moveCoverToHLS(path)

				// 发送成功更新（使用非阻塞发送，防止通道已关闭）
				select {
				case progressChan <- map[string]interface{}{
					"type":      "success",
					"video":     path,
					"timestamp": time.Now().Format(time.RFC3339),
				}:
					// 消息发送成功
				default:
					// 通道已关闭，忽略
				}
			}
		}(i, normalizedPath)
	}

	wg.Wait()

	return total, success, failed, skipped, errors
}

// 首页路由
func indexHandler(c *gin.Context) {
	// 获取showAll参数
	showAll := c.Query("showAll") == "true"

	// 获取动画列表
	var animes []AnimeInfo
	if !localMode {
		animes = getAnimesFromDB()
	}
	if len(animes) == 0 {
		animes = scanVideos()
	}

	// 限制显示数量
	var displayAnimes []AnimeInfo
	if showAll {
		displayAnimes = animes
	} else {
		limit := 20
		if len(animes) > limit {
			displayAnimes = animes[:limit]
		} else {
			displayAnimes = animes
		}
	}

	// 渲染模板
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Animes":      displayAnimes,
		"ShowAll":     showAll,
		"TotalAnimes": len(animes),
	})
}

// 搜索路由
func searchHandler(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	// 搜索动画
	animes := searchAnimes(keyword)

	// 渲染模板
	c.HTML(http.StatusOK, "index.html", gin.H{
		"Animes":      animes,
		"Keyword":     keyword,
		"TotalAnimes": len(animes),
	})
}

// 播放路由
func playHandler(c *gin.Context) {
	// 获取参数
	videoURL := c.Query("video")
	title := c.Query("title")
	summary := c.Query("summary")
	keyword := c.Query("keyword")

	// 解码URL编码的参数
	if keyword != "" {
		decodedKeyword, err := url.QueryUnescape(keyword)
		if err == nil {
			keyword = decodedKeyword
		}
	}

	logger.Printf("=== 播放路由收到请求 ===")
	logger.Printf("videoURL: %s", videoURL)
	logger.Printf("title: %s", title)
	logger.Printf("summary: %s", summary)
	logger.Printf("keyword: %s", keyword)

	if videoURL == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	// 修复：标准化视频URL
	videoURL = normalizeURLPath(videoURL)
	logger.Printf("标准化后的videoURL: %s", videoURL)

	// 从videoURL中提取动画文件夹名称（如果没有keyword参数）
	if keyword == "" {
		logger.Printf("keyword为空，尝试从videoURL中提取")
		// 提取文件夹名称
		pathParts := strings.Split(strings.TrimPrefix(videoURL, "/"), "/")
		logger.Printf("pathParts: %v", pathParts)

		// 查找static部分的索引
		var staticIndex int = -1
		for i, part := range pathParts {
			if part == "static" {
				staticIndex = i
				break
			}
		}
		logger.Printf("staticIndex: %d", staticIndex)

		// 确保找到static部分且后面至少有两个部分
		if staticIndex != -1 && len(pathParts) >= staticIndex+3 {
			logger.Printf("找到static部分，检查后面的路径")
			// 对于/static/videos/动画名称/视频文件.mp4格式
			if pathParts[staticIndex+1] == "videos" {
				keyword = pathParts[staticIndex+2]
				logger.Printf("从videos路径提取的keyword: %s", keyword)
			} else if pathParts[staticIndex+1] == "hls" {
				// 对于/static/hls/动画名称/集数/playlist.m3u8格式
				keyword = pathParts[staticIndex+2]
				logger.Printf("从hls路径提取的keyword: %s", keyword)
			}
		} else {
			logger.Printf("无法从videoURL中提取keyword，staticIndex=%d, len(pathParts)=%d", staticIndex, len(pathParts))
		}
	}

	logger.Printf("最终的keyword: %s", keyword)

	// 获取动画信息
	var anime AnimeInfo
	var found bool
	if keyword != "" {
		logger.Printf("使用keyword '%s' 获取动画信息", keyword)
		anime, found = getAnimeInfo(keyword)
		if found {
			title = anime.Title
			summary = anime.Summary
			logger.Printf("获取到动画信息: Title=%s, Summary=%s", title, summary)
		} else {
			logger.Printf("未找到动画信息")
		}
	} else {
		logger.Printf("keyword为空，无法获取动画信息")
	}

	// 获取动画的所有视频文件
	var videoList []VideoFile
	if keyword != "" {
		logger.Printf("使用keyword '%s' 获取视频列表", keyword)
		videoList = getAnimeVideos(keyword)
		logger.Printf("获取到 %d 个视频文件", len(videoList))
	} else {
		logger.Printf("keyword为空，无法获取视频列表")
	}

	// 检查是否有HLS切片，如果没有则生成
	if !strings.Contains(videoURL, "/hls/") && isVideoFile(videoURL) {
		hlsPath := getHLSURL(videoURL)
		hlsFilePath := strings.TrimPrefix(hlsPath, "/")
		hlsFilePath = filepath.FromSlash(hlsFilePath)

		if _, err := os.Stat(hlsFilePath); os.IsNotExist(err) {
			// 生成HLS切片
			logger.Printf("生成HLS切片: %s\n", videoURL)
			err = generateHLSHighQuality(videoURL)
			if err == nil {
				videoURL = hlsPath
				logger.Printf("HLS切片生成成功: %s\n", hlsPath)
			} else {
				logger.Printf("警告: 生成HLS切片失败: %v\n", err)
			}
		} else {
			// 使用已有的HLS切片
			videoURL = hlsPath
		}
	}

	logger.Printf("准备渲染模板，VideoList长度: %d", len(videoList))

	// 渲染模板
	c.HTML(http.StatusOK, "play.html", gin.H{
		"Title":     title,
		"Summary":   summary,
		"VideoURL":  videoURL,
		"VideoList": videoList,
		"Keyword":   keyword,
		"Cover":     anime.Cover,
	})
}

// 播放记录路由
func historyHandler(c *gin.Context) {
	// 渲染模板
	c.HTML(http.StatusOK, "history.html", gin.H{})
}

// HLS批量生成页面路由
func hlsHandler(c *gin.Context) {
	// 渲染模板
	c.HTML(http.StatusOK, "hls.html", gin.H{})
}

// HLS修复工具页面处理函数
func hlsFixPageHandler(c *gin.Context) {
	// 渲染模板
	c.HTML(http.StatusOK, "hls_fix.html", gin.H{})
}

// 登录页面处理函数
func loginPageHandler(c *gin.Context) {
	// 渲染模板
	c.HTML(http.StatusOK, "login.html", gin.H{})
}

// 注册页面处理函数
func registerPageHandler(c *gin.Context) {
	// 渲染模板
	c.HTML(http.StatusOK, "register.html", gin.H{})
}

// 视频列表API路由
func videoListHandler(c *gin.Context) {
	// 扫描所有视频
	var videos []string
	addedVideos := make(map[string]bool)

	// 1. 扫描原始视频目录
	entries, err := ioutil.ReadDir(videosDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描视频目录失败"})
		return
	}

	// 遍历子目录（每个子目录代表一个动画）
	for _, entry := range entries {
		if entry.IsDir() {
			animeFolder := filepath.Join(videosDir, entry.Name())

			// 扫描动画目录中的视频文件
			videoFiles, err := ioutil.ReadDir(animeFolder)
			if err != nil {
				continue
			}

			for _, file := range videoFiles {
				if !file.IsDir() && isVideoFile(file.Name()) {
					// 构建视频URL路径
					videoPath := normalizeURLPath(path.Join("/", videosDir, entry.Name(), file.Name()))

					// 检查是否有对应的HLS切片
					hlsPath := getHLSURL(videoPath)
					hlsFilePath := strings.TrimPrefix(hlsPath, "/")
					if _, err := os.Stat(hlsFilePath); err == nil {
						// 如果HLS切片存在，使用HLS路径
						videoPath = hlsPath
					}

					// 避免重复添加
					if !addedVideos[videoPath] {
						videos = append(videos, videoPath)
						addedVideos[videoPath] = true
					}
				}
			}
		}
	}

	// 2. 扫描HLS目录，确保即使原始文件被删除，HLS视频也能被发现
	hlsEntries, err := ioutil.ReadDir(hlsDir)
	if err == nil {
		for _, entry := range hlsEntries {
			if entry.IsDir() {
				// 检查是否存在playlist.m3u8文件
				playlistPath := filepath.Join(hlsDir, entry.Name(), "playlist.m3u8")
				if _, err := os.Stat(playlistPath); err == nil {
					// 构建HLS URL路径
					hlsURL := normalizeURLPath(path.Join("/hls", entry.Name(), "playlist.m3u8"))

					// 避免重复添加
					if !addedVideos[hlsURL] {
						videos = append(videos, hlsURL)
						addedVideos[hlsURL] = true
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"videos": videos})
}

// 批量生成HLS切片API路由
func batchHLSHandler(c *gin.Context) {
	// 设置响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 读取请求体
	var request struct {
		Videos []string `json:"videos"`
		UseGPU bool     `json:"useGPU"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.String(http.StatusBadRequest, "data: {\"type\": \"error\", \"message\": \"无效的请求参数\"}\n\n")
		return
	}

	// 检查参数
	if len(request.Videos) == 0 {
		c.String(http.StatusBadRequest, "data: {\"type\": \"error\", \"message\": \"请选择要处理的视频\"}\n\n")
		return
	}

	// 修复：标准化所有请求的视频路径
	normalizedVideos := make([]string, len(request.Videos))
	for i, v := range request.Videos {
		normalizedVideos[i] = normalizeURLPath(v)
	}

	// 生成处理ID
	processID := fmt.Sprintf("%d", time.Now().UnixNano())

	// 创建进度通道和停止通道
	progressChan := make(chan map[string]interface{})
	stopChan := make(chan struct{})

	// 存储处理上下文
	batchProcessingMutex.Lock()
	batchProcessingContexts[processID] = map[string]interface{}{
		"stopChan":     stopChan,
		"progressChan": progressChan,
		"startTime":    time.Now(),
	}
	batchProcessingMutex.Unlock()

	// 发送处理ID
	c.Writer.Header().Set("X-Process-ID", processID)
	c.Writer.Flush()

	// 启动批量处理
	go func() {
		total, success, failed, skipped, errors := batchGenerateHLS(normalizedVideos, request.UseGPU, progressChan, stopChan)

		// 发送完成消息（使用非阻塞发送，防止通道已关闭）
		select {
		case progressChan <- map[string]interface{}{
			"type":      "complete",
			"current":   total,
			"total":     total,
			"success":   success,
			"failed":    failed,
			"skipped":   skipped,
			"errors":    errors,
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}

		// 关闭通道
		close(progressChan)

		// 清理处理上下文
		batchProcessingMutex.Lock()
		delete(batchProcessingContexts, processID)
		batchProcessingMutex.Unlock()
	}()

	// 发送进度更新
	for progress := range progressChan {
		data, err := json.Marshal(progress)
		if err != nil {
			continue
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}

	// 异步增量扫描视频
	go func() {
		// 收集批量处理中涉及的动画目录
		directories := []string{}
		for _, videoPath := range request.Videos {
			dirName := extractAnimeDirectory(videoPath)
			if dirName != "" {
				directories = append(directories, dirName)
			}
		}

		if len(directories) > 0 {
			logger.Println("开始异步增量同步本地动画到数据库...")
			scanAnimeDirectories(directories)
			logger.Println("异步增量同步本地动画到数据库完成！")
		} else {
			logger.Println("批量处理完成，没有需要同步的动画目录")
		}
	}()
}

// 停止批量处理API路由
func stopBatchHLSHandler(c *gin.Context) {
	// 读取处理ID
	processID := c.Query("processId")
	if processID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少处理ID"})
		return
	}

	// 查找处理上下文
	batchProcessingMutex.Lock()
	ctx, exists := batchProcessingContexts[processID]
	batchProcessingMutex.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "处理不存在或已完成"})
		return
	}

	// 发送停止信号
	if stopChan, ok := ctx["stopChan"].(chan struct{}); ok {
		close(stopChan)
	}

	// 清理处理上下文
	batchProcessingMutex.Lock()
	delete(batchProcessingContexts, processID)
	batchProcessingMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "处理已停止"})
}

// 保存播放记录API
func savePlayHistoryHandler(c *gin.Context) {
	var req struct {
		VideoID       string  `json:"videoId"`       // 视频唯一标识
		AnimeTitle    string  `json:"animeTitle"`    // 动画标题
		Episode       string  `json:"episode"`       // 集数名称
		VideoURL      string  `json:"videoUrl"`      // 视频URL
		CurrentTime   float64 `json:"currentTime"`   // 全局播放位置（秒）
		Duration      float64 `json:"duration"`      // 视频总时长（秒）
		Progress      float64 `json:"progress"`      // 播放进度百分比
		SegmentID     string  `json:"segmentId"`     // 当前HLS切片ID（可选）
		SegmentOffset float64 `json:"segmentOffset"` // 在当前切片内的偏移量（秒）
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Printf("错误: 保存播放记录参数错误: %v\n", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}

	// 确保VideoID不为空
	if req.VideoID == "" {
		logger.Println("错误: VideoID为空")
		c.JSON(http.StatusBadRequest, gin.H{"error": "VideoID不能为空"})
		return
	}

	// 获取当前用户ID
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析用户信息
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 获取用户ID
	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 计算进度百分比
	if req.Duration > 0 {
		req.Progress = (req.CurrentTime / req.Duration) * 100
	} else {
		req.Progress = 0
	}

	// 确保CurrentTime不超过Duration
	if req.CurrentTime > req.Duration && req.Duration > 0 {
		req.CurrentTime = req.Duration
		req.Progress = 100
	}

	// 保存到数据库
	if !localMode && db != nil {
		var history PlayHistory
		result := db.Where("video_id = ? AND user_id = ?", req.VideoID, uint(userID)).First(&history)

		if result.Error == nil {
			// 更新现有记录
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

			db.Save(&history)
		} else if result.Error == gorm.ErrRecordNotFound {
			// 创建新记录
			history = PlayHistory{
				UserID:        uint(userID),
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
			db.Create(&history)
		} else {
			logger.Printf("错误: 查询播放记录失败: %v\n", result.Error)
		}
	} else {
		// 使用内存存储（本地模式）
		historyMu.Lock()
		defer historyMu.Unlock()

		// 使用用户ID和视频ID的组合作为键
		historyKey := fmt.Sprintf("%d_%s", uint(userID), req.VideoID)
		existingHistory, exists := historyMap[historyKey]
		if exists {
			// 更新现有记录
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
			// 创建新记录
			historyMap[historyKey] = PlayHistory{
				UserID:        uint(userID),
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

	logger.Printf("播放记录已保存: VideoID=%s, CurrentTime=%.2f, Progress=%.2f%%\n",
		req.VideoID, req.CurrentTime, req.Progress)

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"currentTime": req.CurrentTime,
		"progress":    req.Progress,
	})
}

// 获取播放记录API
func getPlayHistoryHandler(c *gin.Context) {
	videoId := c.Query("videoId")
	if videoId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少videoId"})
		return
	}

	// 获取当前用户ID
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析用户信息
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 获取用户ID
	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var history PlayHistory

	if !localMode && db != nil {
		result := db.Where("video_id = ? AND user_id = ?", videoId, uint(userID)).First(&history)
		if result.Error != nil {
			if result.Error == gorm.ErrRecordNotFound {
				// 无记录则返回默认值
				c.JSON(http.StatusOK, gin.H{
					"videoId":     videoId,
					"currentTime": 0,
					"duration":    0,
					"progress":    0,
					"hasRecord":   false,
				})
				return
			}
			logger.Printf("错误: 查询播放记录失败: %v\n", result.Error)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
			return
		}
	} else {
		// 使用内存存储（本地模式）
		historyMu.Lock()
		historyKey := fmt.Sprintf("%d_%s", uint(userID), videoId)
		existingHistory, exists := historyMap[historyKey]
		historyMu.Unlock()

		if !exists {
			c.JSON(http.StatusOK, gin.H{
				"videoId":     videoId,
				"currentTime": 0,
				"duration":    0,
				"progress":    0,
				"hasRecord":   false,
			})
			return
		}
		history = existingHistory
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

// 获取所有播放记录API
func getAllPlayHistoryHandler(c *gin.Context) {
	// 获取当前用户ID
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析用户信息
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 获取用户ID
	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var histories []PlayHistory

	if !localMode && db != nil {
		// 按最后播放时间降序排列，只获取当前用户的记录
		result := db.Where("user_id = ?", uint(userID)).Order("last_played DESC").Find(&histories)
		if result.Error != nil {
			logger.Printf("错误: 获取所有播放记录失败: %v\n", result.Error)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
			return
		}
	} else {
		// 使用内存存储（本地模式）
		historyMu.Lock()
		defer historyMu.Unlock()

		for _, history := range historyMap {
			// 检查记录是否属于当前用户
			if history.UserID == uint(userID) {
				histories = append(histories, history)
			}
		}
	}

	// 转换为响应格式
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

// 删除单个播放记录API
func deletePlayHistoryHandler(c *gin.Context) {
	videoId := c.Query("videoId")
	if videoId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少videoId"})
		return
	}

	// 获取当前用户ID
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析用户信息
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 获取用户ID
	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if !localMode && db != nil {
		result := db.Where("video_id = ? AND user_id = ?", videoId, uint(userID)).Delete(&PlayHistory{})
		if result.Error != nil {
			logger.Printf("错误: 删除播放记录失败: %v\n", result.Error)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
			return
		}
	} else {
		// 使用内存存储（本地模式）
		historyMu.Lock()
		defer historyMu.Unlock()
		historyKey := fmt.Sprintf("%d_%s", uint(userID), videoId)
		delete(historyMap, historyKey)
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// 清除所有播放记录API
func clearAllPlayHistoryHandler(c *gin.Context) {
	// 获取当前用户ID
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析用户信息
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 获取用户ID
	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	if !localMode && db != nil {
		result := db.Where("user_id = ?", uint(userID)).Delete(&PlayHistory{})
		if result.Error != nil {
			logger.Printf("错误: 清除所有播放记录失败: %v\n", result.Error)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "清除失败"})
			return
		}
	} else {
		// 使用内存存储（本地模式）
		historyMu.Lock()
		defer historyMu.Unlock()

		// 只删除当前用户的记录
		for key, history := range historyMap {
			if history.UserID == uint(userID) {
				delete(historyMap, key)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// 用户注册API
func registerHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "details": err.Error()})
		return
	}

	// 检查用户名是否已存在
	var existingUser User
	result := db.Where("username = ?", req.Username).First(&existingUser)
	if result.Error == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名已存在"})
		return
	}

	// 检查邮箱是否已存在
	result = db.Where("email = ?", req.Email).First(&existingUser)
	if result.Error == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱已存在"})
		return
	}

	// 创建新用户
	user := User{
		Username:  req.Username,
		Email:     req.Email,
		Password:  req.Password, // 实际应用中应该加密密码
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	result = db.Create(&user)
	if result.Error != nil {
		logger.Printf("错误: 创建用户失败: %v\n", result.Error)
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

// 用户登录API
func loginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误", "details": err.Error()})
		return
	}

	// 查找用户
	var user User
	result := db.Where("username = ?", req.Username).First(&user)
	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 验证密码
	if user.Password != req.Password {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	// 设置用户信息到Cookie（简单实现，实际应用中应该加密）
	userInfo := gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	}

	// 将用户信息转换为JSON字符串
	userJSON, err := json.Marshal(userInfo)
	if err == nil {
		// 设置Cookie，有效期为7天
		c.SetCookie(
			"user",
			string(userJSON),
			7*24*60*60, // 7天
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

// 获取当前用户信息API
func getCurrentUserHandler(c *gin.Context) {
	// 从Cookie中获取用户信息
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析Cookie中的用户信息
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

// 用户登出API
func logoutHandler(c *gin.Context) {
	// 清除Cookie
	c.SetCookie(
		"user",
		"",
		-1, // 设置为过期
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

// Cookie自动续签API
func renewCookieHandler(c *gin.Context) {
	// 从Cookie中获取用户信息
	userCookie, err := c.Cookie("user")
	if err != nil || userCookie == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 解析用户信息
	var userInfo map[string]interface{}
	err = json.Unmarshal([]byte(userCookie), &userInfo)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	// 验证用户是否存在
	userID, ok := userInfo["id"].(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	var user User
	result := db.First(&user, uint(userID))
	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户不存在"})
		return
	}

	// 重新设置Cookie，延长有效期
	newUserInfo := gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
	}

	newUserJSON, err := json.Marshal(newUserInfo)
	if err == nil {
		// 设置Cookie，有效期为7天
		c.SetCookie(
			"user",
			string(newUserJSON),
			7*24*60*60, // 7天
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

// 搜索动画API
func searchAnimesHandler(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少搜索关键词"})
		return
	}

	// 搜索动画
	animes := searchAnimes(keyword)

	c.JSON(http.StatusOK, gin.H{"animes": animes})
}

// 删除动画API
func deleteAnimeHandler(c *gin.Context) {
	folderName := c.Query("folderName")
	if folderName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少动画文件夹名称"})
		return
	}

	// 删除文件系统中的动画文件夹
	// 1. 删除原始视频目录
	originalDir := filepath.Join(videosDir, folderName)
	if err := os.RemoveAll(originalDir); err != nil {
		logger.Printf("警告: 删除原始视频目录失败: %v\n", err)
		// 继续执行，不返回错误
	} else {
		logger.Printf("成功删除原始视频目录: %s\n", originalDir)
	}

	// 2. 删除HLS目录
	hlsDirPath := filepath.Join(hlsDir, folderName)
	if err := os.RemoveAll(hlsDirPath); err != nil {
		logger.Printf("警告: 删除HLS目录失败: %v\n", err)
		// 继续执行，不返回错误
	} else {
		logger.Printf("成功删除HLS目录: %s\n", hlsDirPath)
	}

	// 3. 从数据库中删除记录
	if !localMode && db != nil {
		// 删除动画信息
		result := db.Where("folder_name = ?", folderName).Delete(&AnimeInfo{})
		if result.Error != nil {
			logger.Printf("错误: 从数据库删除动画信息失败: %v\n", result.Error)
			// 继续执行，不返回错误
		} else {
			logger.Printf("成功从数据库删除动画信息: %s\n", folderName)
		}

		// 删除相关的播放记录
		result = db.Where("video_url LIKE ?", "%"+folderName+"%").Delete(&PlayHistory{})
		if result.Error != nil {
			logger.Printf("错误: 从数据库删除播放记录失败: %v\n", result.Error)
			// 继续执行，不返回错误
		} else {
			logger.Printf("成功从数据库删除相关播放记录: %s\n", folderName)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

// 扫描视频API
func scanVideosHandler(c *gin.Context) {
	// 异步扫描视频
	go func() {
		logger.Println("开始异步同步本地动画到数据库...")
		animes := scanVideos()
		logger.Printf("异步同步本地动画到数据库完成！扫描到 %d 个动画\n", len(animes))
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "开始扫描视频，请等待完成",
	})
}

// 主函数
func main() {
	// 初始化日志
	initLogger()
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	// 初始化配置
	initConfig()

	// 初始化数据库
	initDB()

	// 确保HLS目录存在
	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		logger.Printf("创建HLS目录: %s\n", hlsDir)
		err = os.MkdirAll(hlsDir, 0755)
		if err != nil {
			logger.Printf("错误: 创建HLS目录失败: %v\n", err)
		}
	}

	// 设置Gin模式
	if config.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin引擎
	r := gin.Default()

	// 加载模板
	r.LoadHTMLGlob(filepath.Join(templatesDir, "*.html"))

	// 静态文件服务
	r.Static("/static", "./static")
	// 添加HLS文件的静态服务
	r.Static("/hls", "./static/hls")

	// 路由
	r.GET("/", indexHandler)
	r.GET("/search", searchHandler)
	r.GET("/play", playHandler)
	r.GET("/history", historyHandler)
	r.GET("/hls", hlsHandler)
	r.GET("/api/videos", videoListHandler)
	r.POST("/api/scan-videos", scanVideosHandler)
	r.POST("/api/batch-hls", batchHLSHandler)
	r.POST("/api/batch-hls/stop", stopBatchHLSHandler)
	// 播放记录相关API
	r.POST("/api/play-history/save", savePlayHistoryHandler)
	r.GET("/api/play-history/get", getPlayHistoryHandler)
	r.GET("/api/play-history/all", getAllPlayHistoryHandler)
	r.DELETE("/api/play-history/delete", deletePlayHistoryHandler)
	r.DELETE("/api/play-history/clear", clearAllPlayHistoryHandler)

	// 用户相关API
	r.POST("/api/auth/register", registerHandler)
	r.POST("/api/auth/login", loginHandler)
	r.GET("/api/auth/user", getCurrentUserHandler)
	r.POST("/api/auth/logout", logoutHandler)
	r.POST("/api/auth/renew", renewCookieHandler)

	// 动画管理相关API
	r.GET("/api/animes/search", searchAnimesHandler)
	r.DELETE("/api/animes/delete", deleteAnimeHandler)
	// 暂时注释掉未定义的函数路由
	r.POST("/api/hls/fix", fixHLSVideosHandler)
	r.GET("/api/hls/fix", fixHLSVideosHandler)
	r.GET("/hls-fix", hlsFixPageHandler)
	r.GET("/login", loginPageHandler)
	r.GET("/register", registerPageHandler)

	// 启动服务器
	port := config.Server.Port
	// 使用IPv6监听地址，确保同时支持IPv4和IPv6
	listenAddr := fmt.Sprintf("[::]:%d", port)
	// 主要IPv6地址（更稳定）
	primaryIPv6Addr := "240e:351:580a:6400:55ab:f75f:cee7:1da1"
	logger.Printf("服务器启动成功！监听端口: %d\n", port)
	logger.Printf("访问地址: http://localhost:%d\n", port)
	logger.Printf("HLS批量生成页面: http://localhost:%d/hls\n", port)
	logger.Printf("IPv6访问地址: http://[%s]:%d\n", primaryIPv6Addr, port)

	// 异步扫描视频
	go func() {
		logger.Println("开始异步同步本地动画到数据库...")
		scanVideos()
		logger.Println("异步同步本地动画到数据库完成！")
	}()

	err := r.Run(listenAddr)
	if err != nil {
		logger.Fatalf("服务器启动失败: %v\n", err)
	}
}
