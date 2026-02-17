package handlers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anime-website/models"
	"anime-website/services"
	"anime-website/utils"

	"github.com/gin-gonic/gin"
)

const (
	videosDir    = "static/videos"
	hlsDir       = "static/hls"
	templatesDir = "templates"
)

var allowedFormats = []string{".mp4", ".flv", ".mkv", ".avi"}

type VideoHandler struct {
	videoService *services.VideoService
}

func NewVideoHandler() *VideoHandler {
	return &VideoHandler{
		videoService: services.VideoServiceInstance,
	}
}

func (h *VideoHandler) Index(c *gin.Context) {
	showAll := c.Query("showAll") == "true"

	var animes []models.AnimeInfo
	if !services.LocalMode {
		animes = h.videoService.GetAnimesFromDB()
	}
	if len(animes) == 0 {
		animes = h.videoService.ScanVideos()
	}

	var displayAnimes []models.AnimeInfo
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

	c.HTML(http.StatusOK, "index.html", gin.H{
		"Animes":      displayAnimes,
		"ShowAll":     showAll,
		"TotalAnimes": len(animes),
	})
}

func (h *VideoHandler) Search(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	animes := h.videoService.SearchAnimes(keyword)

	c.HTML(http.StatusOK, "index.html", gin.H{
		"Animes":      animes,
		"Keyword":     keyword,
		"TotalAnimes": len(animes),
	})
}

func (h *VideoHandler) Play(c *gin.Context) {
	videoURL := c.Query("video")
	title := c.Query("title")
	summary := c.Query("summary")
	keyword := c.Query("keyword")

	if keyword != "" {
		decodedKeyword, err := url.QueryUnescape(keyword)
		if err == nil {
			keyword = decodedKeyword
		}
	}

	log.Printf("=== 播放路由收到请求 ===")
	log.Printf("videoURL: %s", videoURL)
	log.Printf("title: %s", title)
	log.Printf("summary: %s", summary)
	log.Printf("keyword: %s", keyword)

	if videoURL == "" {
		c.Redirect(http.StatusFound, "/")
		return
	}

	var anime models.AnimeInfo
	var found bool

	if keyword != "" {
		log.Printf("使用keyword '%s' 获取动画信息", keyword)
		anime, found = h.videoService.GetAnimeInfo(keyword)
		if found {
			title = anime.Title
			summary = anime.Summary
			log.Printf("获取到动画信息: Title=%s, Summary=%s, PhysicalPath=%s, StorageDisk=%s", title, summary, anime.PhysicalPath, anime.StorageDisk)
		} else {
			log.Printf("未找到动画信息")
		}
	}

	var videoList []models.VideoFile
	if keyword != "" {
		log.Printf("使用keyword '%s' 获取视频列表", keyword)
		videoList = h.videoService.GetAnimeVideos(keyword)
		log.Printf("获取到 %d 个视频文件", len(videoList))
	}

	log.Printf("检查videoURL是否为HLS格式: %s", videoURL)

	if !strings.Contains(videoURL, ".m3u8") && utils.IsVideoFile(videoURL, allowedFormats) {
		log.Printf("视频URL不是HLS格式，需要生成HLS: %s", videoURL)

		var hlsPath string
		if found && anime.PhysicalPath != "" {
			hlsPath = h.videoService.GenerateHLSFromPhysicalPath(videoURL, anime.PhysicalPath, anime.StorageDisk)
			log.Printf("使用物理路径生成HLS URL: %s", hlsPath)
		} else {
			hlsPath = h.videoService.GetHLSURL(videoURL)
			log.Printf("使用默认方式生成HLS URL: %s", hlsPath)
		}

		hlsFilePath := strings.TrimPrefix(hlsPath, "/")
		hlsFilePath = filepath.FromSlash(hlsFilePath)
		log.Printf("HLS文件物理路径: %s", hlsFilePath)

		if _, err := os.Stat(hlsFilePath); os.IsNotExist(err) {
			log.Printf("HLS文件不存在，开始生成: %s\n", videoURL)
			err = h.videoService.GenerateHLSHighQuality(videoURL)
			if err == nil {
				videoURL = hlsPath
				log.Printf("HLS切片生成成功: %s\n", hlsPath)
			} else {
				log.Printf("警告: 生成HLS切片失败: %v\n", err)
			}
		} else {
			videoURL = hlsPath
			log.Printf("使用已存在的HLS文件: %s\n", hlsPath)
		}
	} else if strings.Contains(videoURL, ".m3u8") {
		log.Printf("videoURL已经是HLS格式，直接使用: %s", videoURL)

		if strings.HasPrefix(videoURL, "/storage/") {
			relativePath := strings.TrimPrefix(videoURL, "/storage/")
			pathParts := strings.Split(relativePath, "/")
			if len(pathParts) >= 2 {
				diskName := pathParts[0]
				disk := services.StorageServiceInstance.GetDiskByName(diskName)
				if disk != nil {
					physicalPath := filepath.Join(disk.Path, strings.Join(pathParts[1:], string(filepath.Separator)))
					log.Printf("HLS文件物理路径: %s", physicalPath)

					if _, err := os.Stat(physicalPath); os.IsNotExist(err) {
						log.Printf("警告: HLS文件不存在: %s", physicalPath)
					} else {
						log.Printf("HLS文件存在，可以使用")
					}
				} else {
					log.Printf("警告: 找不到磁盘: %s", diskName)
				}
			}
		} else {
			hlsFilePath := strings.TrimPrefix(videoURL, "/")
			hlsFilePath = filepath.FromSlash(hlsFilePath)
			log.Printf("HLS文件物理路径: %s", hlsFilePath)

			if _, err := os.Stat(hlsFilePath); os.IsNotExist(err) {
				log.Printf("警告: HLS文件不存在: %s", hlsFilePath)
			} else {
				log.Printf("HLS文件存在，可以使用")
			}
		}
	}

	log.Printf("最终使用的VideoURL: %s", videoURL)
	log.Printf("准备渲染模板，VideoList长度: %d", len(videoList))

	c.HTML(http.StatusOK, "play.html", gin.H{
		"Title":     title,
		"Summary":   summary,
		"VideoURL":  videoURL,
		"VideoList": videoList,
		"Keyword":   keyword,
		"Cover":     anime.Cover,
	})
}

func (h *VideoHandler) History(c *gin.Context) {
	c.HTML(http.StatusOK, "history.html", gin.H{})
}

func (h *VideoHandler) HLS(c *gin.Context) {
	c.HTML(http.StatusOK, "hls.html", gin.H{})
}

func (h *VideoHandler) HLSFix(c *gin.Context) {
	c.HTML(http.StatusOK, "hls_fix.html", gin.H{})
}

func (h *VideoHandler) VideoList(c *gin.Context) {
	var videos []string
	addedVideos := make(map[string]bool)

	entries, err := ioutil.ReadDir(videosDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "扫描视频目录失败"})
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			animeFolder := filepath.Join(videosDir, entry.Name())

			videoFiles, err := ioutil.ReadDir(animeFolder)
			if err != nil {
				continue
			}

			for _, file := range videoFiles {
				if !file.IsDir() && utils.IsVideoFile(file.Name(), allowedFormats) {
					videoPath := utils.NormalizeURLPath(strings.Join([]string{"/", videosDir, entry.Name(), file.Name()}, "/"))

					hlsPath := h.videoService.GetHLSURL(videoPath)
					hlsFilePath := strings.TrimPrefix(hlsPath, "/")
					if _, err := os.Stat(hlsFilePath); err == nil {
						videoPath = hlsPath
					}

					if !addedVideos[videoPath] {
						videos = append(videos, videoPath)
						addedVideos[videoPath] = true
					}
				}
			}
		}
	}

	hlsEntries, err := ioutil.ReadDir(hlsDir)
	if err == nil {
		for _, entry := range hlsEntries {
			if entry.IsDir() {
				playlistPath := filepath.Join(hlsDir, entry.Name(), "playlist.m3u8")
				if _, err := os.Stat(playlistPath); err == nil {
					hlsURL := utils.NormalizeURLPath(strings.Join([]string{"/hls", entry.Name(), "playlist.m3u8"}, "/"))

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

func (h *VideoHandler) BatchHLS(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var request struct {
		Videos []string `json:"videos"`
		UseGPU bool     `json:"useGPU"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.String(http.StatusBadRequest, "data: {\"type\": \"error\", \"message\": \"无效的请求参数\"}\n\n")
		return
	}

	if len(request.Videos) == 0 {
		c.String(http.StatusBadRequest, "data: {\"type\": \"error\", \"message\": \"请选择要处理的视频\"}\n\n")
		return
	}

	normalizedVideos := make([]string, len(request.Videos))
	for i, v := range request.Videos {
		normalizedVideos[i] = utils.NormalizeURLPath(v)
	}

	processID := fmt.Sprintf("%d", time.Now().UnixNano())

	progressChan := make(chan map[string]interface{})
	stopChan := make(chan struct{})

	services.BatchProcessingMutex.Lock()
	services.BatchProcessingContexts[processID] = map[string]interface{}{
		"stopChan":     stopChan,
		"progressChan": progressChan,
		"startTime":    time.Now(),
	}
	services.BatchProcessingMutex.Unlock()

	c.Writer.Header().Set("X-Process-ID", processID)
	c.Writer.Flush()

	go func() {
		total, success, failed, skipped, errors := h.videoService.BatchGenerateHLS(normalizedVideos, request.UseGPU, progressChan, stopChan)

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
		default:
		}

		close(progressChan)

		services.BatchProcessingMutex.Lock()
		delete(services.BatchProcessingContexts, processID)
		services.BatchProcessingMutex.Unlock()
	}()

	for progress := range progressChan {
		data, err := json.Marshal(progress)
		if err != nil {
			continue
		}
		fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		c.Writer.Flush()
	}

	go func() {
		directories := []string{}
		for _, videoPath := range request.Videos {
			dirName := h.videoService.ExtractAnimeDirectory(videoPath)
			if dirName != "" {
				directories = append(directories, dirName)
			}
		}

		if len(directories) > 0 {
			log.Println("开始异步增量同步本地动画到数据库...")
			h.videoService.ScanAnimeDirectories(directories)
			log.Println("异步增量同步本地动画到数据库完成！")
		} else {
			log.Println("批量处理完成，没有需要同步的动画目录")
		}
	}()
}

func (h *VideoHandler) StopBatchHLS(c *gin.Context) {
	processID := c.Query("processId")
	if processID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少处理ID"})
		return
	}

	services.BatchProcessingMutex.Lock()
	ctx, exists := services.BatchProcessingContexts[processID]
	services.BatchProcessingMutex.Unlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "处理不存在或已完成"})
		return
	}

	if stopChan, ok := ctx["stopChan"].(chan struct{}); ok {
		close(stopChan)
	}

	services.BatchProcessingMutex.Lock()
	delete(services.BatchProcessingContexts, processID)
	services.BatchProcessingMutex.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "处理已停止"})
}

func (h *VideoHandler) ScanVideos(c *gin.Context) {
	go func() {
		log.Println("开始异步同步本地动画到数据库...")
		animes := h.videoService.ScanVideos()
		log.Printf("异步同步本地动画到数据库完成！扫描到 %d 个动画\n", len(animes))
	}()

	if c != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "开始扫描视频，请等待完成",
		})
	}
}

func (h *VideoHandler) SearchAnimes(c *gin.Context) {
	keyword := c.Query("keyword")
	if keyword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少搜索关键词"})
		return
	}

	animes := h.videoService.SearchAnimes(keyword)

	c.JSON(http.StatusOK, gin.H{"animes": animes})
}

func (h *VideoHandler) DeleteAnime(c *gin.Context) {
	folderName := c.Query("folderName")
	if folderName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少动画文件夹名称"})
		return
	}

	err := h.videoService.DeleteAnime(folderName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *VideoHandler) FixHLSVideos(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "HLS修复功能待实现"})
}

func (h *VideoHandler) LoginPage(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", gin.H{})
}

func (h *VideoHandler) RegisterPage(c *gin.Context) {
	c.HTML(http.StatusOK, "register.html", gin.H{})
}
