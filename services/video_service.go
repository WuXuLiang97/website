package services

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"anime-website/models"
	"anime-website/utils"

	"gorm.io/gorm"
)

const (
	videosDir  = "static/videos"
	hlsDir     = "static/hls"
	maxWorkers = 5
)

var allowedFormats = []string{".mp4", ".flv", ".mkv", ".avi"}

type VideoService struct{}

var VideoServiceInstance = &VideoService{}

var historyMap = make(map[string]models.PlayHistory)
var historyMu sync.Mutex
var movedCoverDirs = make(map[string]bool)
var movedCoverDirsMutex sync.Mutex
var BatchProcessingContexts = make(map[string]map[string]interface{})
var BatchProcessingMutex sync.Mutex

func (s *VideoService) ScanVideos() []models.AnimeInfo {
	var animes []models.AnimeInfo
	var mutex sync.Mutex
	var wg sync.WaitGroup

	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		log.Printf("警告: HLS目录 %s 不存在，将创建\n", hlsDir)
		err = os.MkdirAll(hlsDir, 0755)
		if err != nil {
			log.Printf("错误: 创建HLS目录失败: %v\n", err)
		}
	}

	disks := StorageServiceInstance.GetAllDisks()
	if len(disks) == 0 {
		entries, err := ioutil.ReadDir(hlsDir)
		if err != nil {
			log.Printf("错误: 扫描HLS目录失败: %v\n", err)
			return animes
		}

		for _, entry := range entries {
			if entry.IsDir() {
				animeName := entry.Name()
				hlsAnimePath := filepath.Join(hlsDir, animeName)
				wg.Add(1)
				go s.scanAnimeDirectory(animeName, hlsAnimePath, hlsDir, &mutex, &wg, &animes)
			}
		}
	} else {
		for _, disk := range disks {
			if !disk.Enabled {
				continue
			}
			entries, err := ioutil.ReadDir(disk.Path)
			if err != nil {
				log.Printf("警告: 扫描磁盘 %s 失败: %v\n", disk.Name, err)
				continue
			}

			for _, entry := range entries {
				if entry.IsDir() {
					animeName := entry.Name()
					hlsAnimePath := filepath.Join(disk.Path, animeName)
					wg.Add(1)
					go s.scanAnimeDirectory(animeName, hlsAnimePath, disk.Path, &mutex, &wg, &animes)
				}
			}
		}
	}

	wg.Wait()

	sort.Slice(animes, func(i, j int) bool {
		return animes[i].Title < animes[j].Title
	})

	return animes
}

func (s *VideoService) scanAnimeDirectory(animeName string, hlsAnimePath string, basePath string, mutex *sync.Mutex, wg *sync.WaitGroup, animes *[]models.AnimeInfo) {
	defer wg.Done()

	var videos []models.VideoFile
	var diskName string

	disk := StorageServiceInstance.FindDiskByAnimeName(animeName)
	if disk != nil {
		diskName = disk.Name
	}

	hlsEntries, err := ioutil.ReadDir(hlsAnimePath)
	if err == nil {
		for _, hlsEntry := range hlsEntries {
			if hlsEntry.IsDir() {
				playlistPath := filepath.Join(hlsAnimePath, hlsEntry.Name(), "playlist.m3u8")
				if _, err := os.Stat(playlistPath); err == nil {
					var hlsURL string
					var physicalPath string

					if basePath == hlsDir {
						hlsURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", animeName, hlsEntry.Name(), "playlist.m3u8"}, "/"))
						physicalPath = playlistPath
					} else if disk != nil {
						hlsURL = "/storage/" + disk.Name + "/" + animeName + "/" + hlsEntry.Name() + "/playlist.m3u8"
						physicalPath = playlistPath
					} else {
						hlsURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", animeName, hlsEntry.Name(), "playlist.m3u8"}, "/"))
						physicalPath = playlistPath
					}

					videos = append(videos, models.VideoFile{
						Path:         hlsURL,
						FileName:     hlsEntry.Name(),
						PhysicalPath: physicalPath,
					})
				}
			}
		}
	}

	if len(videos) > 0 {
		sort.Slice(videos, func(i, j int) bool {
			return videos[i].FileName < videos[j].FileName
		})

		mainVideo := videos[0]
		coverURL := "/static/css/default-cover.jpg"
		coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

		for _, format := range coverFormats {
			hlsCoverPath := filepath.Join(basePath, animeName, format)
			if _, err := os.Stat(hlsCoverPath); err == nil {
				if basePath == hlsDir {
					coverURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", animeName, format}, "/"))
				} else if disk != nil {
					coverURL = "/storage/" + disk.Name + "/" + animeName + "/" + format
				} else {
					coverURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", animeName, format}, "/"))
				}
				break
			}
		}

		anime := models.AnimeInfo{
			Title:        animeName,
			Summary:      fmt.Sprintf("这是一部名为 %s 的动画", animeName),
			Cover:        coverURL,
			VideoURL:     mainVideo.Path,
			Episodes:     len(videos),
			FolderName:   animeName,
			PhysicalPath: hlsAnimePath,
			StorageDisk:  diskName,
		}

		mutex.Lock()
		*animes = append(*animes, anime)
		mutex.Unlock()

		if !LocalMode {
			s.updateAnimeInfo(anime)
		}
	}
}

func (s *VideoService) updateAnimeInfo(anime models.AnimeInfo) {
	var existingAnime models.AnimeInfo
	result := DB.Where("folder_name = ?", anime.FolderName).First(&existingAnime)

	if result.Error == nil {
		log.Printf("更新动画信息: %s\n", anime.FolderName)
		existingAnime.Title = anime.Title
		existingAnime.Summary = anime.Summary
		existingAnime.Cover = anime.Cover
		existingAnime.VideoURL = anime.VideoURL
		existingAnime.Episodes = anime.Episodes
		existingAnime.PhysicalPath = anime.PhysicalPath
		existingAnime.StorageDisk = anime.StorageDisk
		existingAnime.UpdatedAt = time.Now()

		result = DB.Save(&existingAnime)
		if result.Error != nil {
			log.Printf("错误: 更新动画信息失败: %v\n", result.Error)
		} else {
			log.Printf("成功更新动画信息: %s\n", anime.FolderName)
		}
	} else if result.Error == gorm.ErrRecordNotFound {
		log.Printf("创建新动画: %s\n", anime.FolderName)
		anime.CreatedAt = time.Now()
		anime.UpdatedAt = time.Now()

		result = DB.Create(&anime)
		if result.Error != nil {
			log.Printf("错误: 创建动画信息失败: %v\n", result.Error)
		} else {
			log.Printf("成功创建动画信息: %s\n", anime.FolderName)
		}
	} else {
		log.Printf("错误: 查询动画信息失败: %v\n", result.Error)
	}
}

func (s *VideoService) GetAnimesFromDB() []models.AnimeInfo {
	var animes []models.AnimeInfo
	result := DB.Find(&animes)
	if result.Error != nil {
		log.Printf("错误: 从数据库获取动画信息失败: %v\n", result.Error)
		return s.ScanVideos()
	}

	for i := range animes {
		currentCoverPath := strings.TrimPrefix(animes[i].Cover, "/")
		if _, err := os.Stat(currentCoverPath); os.IsNotExist(err) {
			coverURL := "/static/css/default-cover.jpg"
			coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

			for _, format := range coverFormats {
				hlsCoverPath := filepath.Join(hlsDir, animes[i].FolderName, format)
				if _, err := os.Stat(hlsCoverPath); err == nil {
					coverURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", animes[i].FolderName, format}, "/"))
					break
				}
				coverPath := filepath.Join(videosDir, animes[i].FolderName, format)
				if _, err := os.Stat(coverPath); err == nil {
					coverURL = utils.NormalizeURLPath(strings.Join([]string{"/", videosDir, animes[i].FolderName, format}, "/"))
					break
				}
			}

			if coverURL == "/static/css/default-cover.jpg" {
				disks := StorageServiceInstance.GetAllDisks()
				for _, disk := range disks {
					if !disk.Enabled {
						continue
					}
					for _, format := range coverFormats {
						diskCoverPath := filepath.Join(disk.Path, animes[i].FolderName, format)
						if _, err := os.Stat(diskCoverPath); err == nil {
							coverURL = "/storage/" + disk.Name + "/" + animes[i].FolderName + "/" + format
							break
						}
					}
					if coverURL != "/static/css/default-cover.jpg" {
						break
					}
				}
			}

			animes[i].Cover = coverURL
			DB.Model(&animes[i]).Update("cover", coverURL)
		}
	}

	return animes
}

func (s *VideoService) SearchAnimes(keyword string) []models.AnimeInfo {
	var animes []models.AnimeInfo

	if !LocalMode {
		result := DB.Where("title LIKE ? OR folder_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%").Find(&animes)
		if result.Error != nil {
			log.Printf("错误: 搜索动画失败: %v\n", result.Error)
			animes = s.ScanVideos()
		} else {
			return animes
		}
	}

	allAnimes := s.ScanVideos()
	for _, anime := range allAnimes {
		if strings.Contains(strings.ToLower(anime.Title), strings.ToLower(keyword)) ||
			strings.Contains(strings.ToLower(anime.FolderName), strings.ToLower(keyword)) {
			animes = append(animes, anime)
		}
	}

	return animes
}

func (s *VideoService) GetAnimeInfo(folderName string) (models.AnimeInfo, bool) {
	var anime models.AnimeInfo

	if !LocalMode {
		result := DB.Where("folder_name = ?", folderName).First(&anime)
		if result.Error == nil {
			return anime, true
		} else if result.Error != nil {
			log.Printf("错误: 获取动画信息失败: %v\n", result.Error)
		}
	}

	animeFolder := filepath.Join(videosDir, folderName)
	var videos []models.VideoFile
	var hasVideoFiles bool

	if _, err := os.Stat(animeFolder); err == nil {
		videoFiles, err := ioutil.ReadDir(animeFolder)
		if err == nil {
			for _, file := range videoFiles {
				if !file.IsDir() && utils.IsVideoFile(file.Name(), allowedFormats) {
					videoPath := utils.NormalizeURLPath(strings.Join([]string{"/", videosDir, folderName, file.Name()}, "/"))
					videos = append(videos, models.VideoFile{
						Path:     videoPath,
						FileName: file.Name(),
					})
					hasVideoFiles = true
				}
			}
		}
	}

	hlsFolder := filepath.Join(hlsDir, folderName)
	log.Printf("检查HLS文件夹: %s, hasVideoFiles=%v\n", hlsFolder, hasVideoFiles)
	if !hasVideoFiles {
		if _, err := os.Stat(hlsFolder); err == nil {
			log.Printf("HLS文件夹存在，开始扫描: %s\n", hlsFolder)
			hlsEntries, err := ioutil.ReadDir(hlsFolder)
			if err == nil {
				log.Printf("找到 %d 个条目\n", len(hlsEntries))
				for _, entry := range hlsEntries {
					if entry.IsDir() {
						playlistPath := filepath.Join(hlsFolder, entry.Name(), "playlist.m3u8")
						if _, err := os.Stat(playlistPath); err == nil {
							hlsURL := utils.NormalizeURLPath(strings.Join([]string{"/hls", folderName, entry.Name(), "playlist.m3u8"}, "/"))
							videos = append(videos, models.VideoFile{
								Path:     hlsURL,
								FileName: entry.Name(),
							})
							hasVideoFiles = true
							log.Printf("找到视频文件: %s, URL: %s\n", entry.Name(), hlsURL)
						}
					}
				}
				log.Printf("扫描完成，共找到 %d 个视频文件\n", len(videos))
			} else {
				log.Printf("读取HLS文件夹失败: %v\n", err)
			}
		} else {
			log.Printf("HLS文件夹不存在: %s, 错误: %v\n", hlsFolder, err)
		}

		disks := StorageServiceInstance.GetAllDisks()
		for _, disk := range disks {
			if !disk.Enabled {
				continue
			}
			diskHlsPath := filepath.Join(disk.Path, folderName)
			if _, err := os.Stat(diskHlsPath); err == nil {
				log.Printf("磁盘 %s HLS文件夹存在，开始扫描: %s\n", disk.Name, diskHlsPath)
				hlsEntries, err := ioutil.ReadDir(diskHlsPath)
				if err == nil {
					for _, entry := range hlsEntries {
						if entry.IsDir() {
							playlistPath := filepath.Join(diskHlsPath, entry.Name(), "playlist.m3u8")
							if _, err := os.Stat(playlistPath); err == nil {
								hlsURL := "/storage/" + disk.Name + "/" + folderName + "/" + entry.Name() + "/playlist.m3u8"
								videos = append(videos, models.VideoFile{
									Path:     hlsURL,
									FileName: entry.Name(),
								})
								hasVideoFiles = true
								log.Printf("找到视频文件: %s, URL: %s\n", entry.Name(), hlsURL)
							}
						}
					}
				}
			}
		}
	}

	if hasVideoFiles && len(videos) > 0 {
		sort.Slice(videos, func(i, j int) bool {
			return videos[i].FileName < videos[j].FileName
		})

		mainVideo := videos[0]
		coverURL := "/static/css/default-cover.jpg"
		coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

		for _, format := range coverFormats {
			hlsCoverPath := filepath.Join(hlsDir, folderName, format)
			if _, err := os.Stat(hlsCoverPath); err == nil {
				coverURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", folderName, format}, "/"))
				break
			}
			coverPath := filepath.Join(animeFolder, format)
			if _, err := os.Stat(coverPath); err == nil {
				coverURL = utils.NormalizeURLPath(strings.Join([]string{"/", videosDir, folderName, format}, "/"))
				break
			}
		}

		if coverURL == "/static/css/default-cover.jpg" {
			disks := StorageServiceInstance.GetAllDisks()
			for _, disk := range disks {
				if !disk.Enabled {
					continue
				}
				for _, format := range coverFormats {
					diskCoverPath := filepath.Join(disk.Path, folderName, format)
					if _, err := os.Stat(diskCoverPath); err == nil {
						coverURL = "/storage/" + disk.Name + "/" + folderName + "/" + format
						break
					}
				}
				if coverURL != "/static/css/default-cover.jpg" {
					break
				}
			}
		}

		anime = models.AnimeInfo{
			Title:      folderName,
			Summary:    fmt.Sprintf("这是一部名为 %s 的动画", folderName),
			Cover:      coverURL,
			VideoURL:   mainVideo.Path,
			Episodes:   len(videos),
			FolderName: folderName,
		}

		if !strings.Contains(mainVideo.Path, "/hls/") {
			hlsPath := s.getHLSURL(mainVideo.Path)
			if _, err := os.Stat(strings.TrimPrefix(hlsPath, "/")); err == nil {
				anime.VideoURL = hlsPath
			}
		}

		return anime, true
	}

	return anime, false
}

func (s *VideoService) GetAnimeVideos(folderName string) []models.VideoFile {
	var videos []models.VideoFile
	addedVideos := make(map[string]bool)

	disks := StorageServiceInstance.GetAllDisks()
	if len(disks) == 0 {
		hlsAnimePath := filepath.Join(hlsDir, folderName)
		if _, err := os.Stat(hlsAnimePath); err == nil {
			hlsEntries, err := ioutil.ReadDir(hlsAnimePath)
			if err == nil {
				for _, entry := range hlsEntries {
					if entry.IsDir() {
						playlistPath := filepath.Join(hlsAnimePath, entry.Name(), "playlist.m3u8")
						if _, err := os.Stat(playlistPath); err == nil {
							hlsURL := utils.NormalizeURLPath(strings.Join([]string{"/hls", folderName, entry.Name(), "playlist.m3u8"}, "/"))
							hlsFileName := entry.Name()

							if !addedVideos[hlsURL] {
								videos = append(videos, models.VideoFile{
									Path:         hlsURL,
									FileName:     hlsFileName,
									PhysicalPath: playlistPath,
								})
								addedVideos[hlsURL] = true
							}
						}
					}
				}
			}
		}
	} else {
		for _, disk := range disks {
			if !disk.Enabled {
				continue
			}
			diskHlsPath := filepath.Join(disk.Path, folderName)
			if _, err := os.Stat(diskHlsPath); err == nil {
				hlsEntries, err := ioutil.ReadDir(diskHlsPath)
				if err == nil {
					for _, entry := range hlsEntries {
						if entry.IsDir() {
							playlistPath := filepath.Join(diskHlsPath, entry.Name(), "playlist.m3u8")
							if _, err := os.Stat(playlistPath); err == nil {
								hlsURL := "/storage/" + disk.Name + "/" + folderName + "/" + entry.Name() + "/playlist.m3u8"
								hlsFileName := entry.Name()

								if !addedVideos[hlsURL] {
									videos = append(videos, models.VideoFile{
										Path:         hlsURL,
										FileName:     hlsFileName,
										PhysicalPath: playlistPath,
									})
									addedVideos[hlsURL] = true
								}
							}
						}
					}
				}
			}
		}
	}

	sort.Slice(videos, func(i, j int) bool {
		return videos[i].FileName < videos[j].FileName
	})

	return videos
}

func (s *VideoService) getHLSDir(videoPath string) string {
	normalizedPath := utils.NormalizeURLPath(videoPath)
	relativePath := strings.TrimPrefix(normalizedPath, "/static/videos/")
	pathParts := strings.Split(relativePath, "/")
	if len(pathParts) < 1 {
		return filepath.Join(hlsDir, filepath.Base(relativePath))
	}
	animeName := pathParts[0]
	return StorageServiceInstance.GetHLSPath(animeName)
}

func (s *VideoService) getHLSURL(videoPath string) string {
	normalizedPath := utils.NormalizeURLPath(videoPath)
	relativePath := strings.TrimPrefix(normalizedPath, "/static/videos/")
	pathParts := strings.Split(relativePath, "/")
	if len(pathParts) < 1 {
		return utils.NormalizeURLPath(strings.Join([]string{"/hls", filepath.Base(relativePath), "playlist.m3u8"}, "/"))
	}
	animeName := pathParts[0]
	return StorageServiceInstance.GetHLSURL(animeName) + "/playlist.m3u8"
}

func (s *VideoService) getVideoFilePath(videoPath string) string {
	normalizedPath := utils.NormalizeURLPath(videoPath)

	if strings.HasPrefix(normalizedPath, "/storage/") {
		relativePath := strings.TrimPrefix(normalizedPath, "/storage/")
		pathParts := strings.Split(relativePath, "/")
		if len(pathParts) >= 2 {
			diskName := pathParts[0]
			disk := StorageServiceInstance.GetDiskByName(diskName)
			if disk != nil {
				return filepath.Join(disk.Path, strings.Join(pathParts[1:], string(filepath.Separator)))
			}
		}
		return filepath.FromSlash(strings.TrimPrefix(normalizedPath, "/"))
	}

	videoFilePath := strings.TrimPrefix(normalizedPath, "/")
	return filepath.FromSlash(videoFilePath)
}

func (s *VideoService) GenerateHLSFromPhysicalPath(videoURL string, physicalPath string, storageDisk string) string {
	if physicalPath == "" {
		return s.GetHLSURL(videoURL)
	}

	animeName := filepath.Base(physicalPath)

	if storageDisk != "" {
		disk := StorageServiceInstance.GetDiskByName(storageDisk)
		if disk != nil {
			return "/storage/" + disk.Name + "/" + animeName + "/playlist.m3u8"
		}
	}

	return "/hls/" + animeName + "/playlist.m3u8"
}

func (s *VideoService) GenerateHLS(videoPath string) error {
	hlsDirPath := s.getHLSDir(videoPath)

	err := os.MkdirAll(hlsDirPath, 0755)
	if err != nil {
		return fmt.Errorf("创建HLS目录失败: %v", err)
	}

	playlistPath := filepath.Join(hlsDirPath, "playlist.m3u8")
	segmentPath := filepath.Join(hlsDirPath, "segment_%03d.ts")

	videoFilePath := s.getVideoFilePath(videoPath)

	cmd := exec.Command(
		"ffmpeg",
		"-err_detect", "ignore_err",
		"-i", videoFilePath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-hls_time", "8",
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPath,
		"-hls_flags", "discont_start+temp_file+independent_segments",
		"-avoid_negative_ts", "make_zero",
		"-fflags", "+genpts+igndts",
		"-reset_timestamps", "1",
		"-loglevel", "error",
		playlistPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行FFmpeg命令失败: %v, 输出: %s", err, string(output))
	}

	if strings.HasSuffix(strings.ToLower(videoFilePath), ".mp4") {
		err = os.Remove(videoFilePath)
		if err != nil {
			log.Printf("警告: 删除原始文件失败: %v\n", err)
		} else {
			log.Printf("成功删除原始文件: %s\n", videoFilePath)
		}
	}

	return nil
}

func (s *VideoService) GenerateHLSHighQuality(videoPath string) error {
	hlsDirPath := s.getHLSDir(videoPath)

	err := os.MkdirAll(hlsDirPath, 0755)
	if err != nil {
		return fmt.Errorf("创建HLS目录失败: %v", err)
	}

	playlistPath := filepath.Join(hlsDirPath, "playlist.m3u8")
	segmentPath := filepath.Join(hlsDirPath, "segment_%03d.ts")

	videoFilePath := s.getVideoFilePath(videoPath)

	cmd := exec.Command(
		"ffmpeg",
		"-err_detect", "ignore_err",
		"-i", videoFilePath,
		"-c:v", "copy",
		"-c:a", "copy",
		"-hls_time", "8",
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPath,
		"-hls_flags", "discont_start+temp_file+independent_segments",
		"-avoid_negative_ts", "make_zero",
		"-fflags", "+genpts+igndts",
		"-reset_timestamps", "1",
		"-loglevel", "error",
		playlistPath,
	)

	_, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("警告: GPU加速失败，尝试使用CPU: %v\n", err)
		return s.GenerateHLS(videoPath)
	}

	if strings.HasSuffix(strings.ToLower(videoFilePath), ".mp4") {
		err = os.Remove(videoFilePath)
		if err != nil {
			log.Printf("警告: 删除原始文件失败: %v\n", err)
		} else {
			log.Printf("成功删除原始文件: %s\n", videoFilePath)
		}
	}

	return nil
}

func (s *VideoService) BatchGenerateHLS(videos []string, useGPU bool, progressChan chan<- map[string]interface{}, stopChan <-chan struct{}) (int, int, int, int, []string) {
	total := len(videos)
	success := 0
	failed := 0
	skipped := 0
	var errors []string

	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, videoPath := range videos {
		select {
		case <-stopChan:
			select {
			case progressChan <- map[string]interface{}{
				"type":      "stop",
				"message":   "处理已停止",
				"timestamp": time.Now().Format(time.RFC3339),
			}:
			default:
			}
			wg.Wait()
			return total, success, failed, skipped, errors
		default:
		}

		normalizedPath := utils.NormalizeURLPath(videoPath)

		hlsPath := s.getHLSURL(normalizedPath)
		hlsFilePath := strings.TrimPrefix(hlsPath, "/")

		if _, err := os.Stat(hlsFilePath); err == nil {
			skipped++
			select {
			case progressChan <- map[string]interface{}{
				"type":      "skipped",
				"video":     normalizedPath,
				"message":   "HLS切片已存在，跳过处理",
				"timestamp": time.Now().Format(time.RFC3339),
			}:
			default:
			}
			continue
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(idx int, path string) {
			defer func() {
				wg.Done()
				<-semaphore
			}()

			select {
			case progressChan <- map[string]interface{}{
				"type":      "progress",
				"current":   idx + 1,
				"total":     total,
				"video":     path,
				"status":    "processing",
				"timestamp": time.Now().Format(time.RFC3339),
			}:
			default:
			}

			var err error
			if useGPU {
				err = s.GenerateHLSHighQuality(path)
			} else {
				err = s.GenerateHLS(path)
			}

			if err != nil {
				failed++
				errorMsg := fmt.Sprintf("视频 %s 生成失败: %v", path, err)
				errors = append(errors, errorMsg)
				log.Printf("错误: %s\n", errorMsg)

				select {
				case progressChan <- map[string]interface{}{
					"type":      "error",
					"video":     path,
					"message":   err.Error(),
					"timestamp": time.Now().Format(time.RFC3339),
				}:
				default:
				}
			} else {
				success++
				log.Printf("成功: 视频 %s 生成HLS切片完成\n", path)

				s.moveCoverToHLS(path)

				select {
				case progressChan <- map[string]interface{}{
					"type":      "success",
					"video":     path,
					"timestamp": time.Now().Format(time.RFC3339),
				}:
				default:
				}
			}
		}(i, normalizedPath)
	}

	wg.Wait()

	return total, success, failed, skipped, errors
}

func (s *VideoService) moveCoverToHLS(videoPath string) {
	normalizedPath := utils.NormalizeURLPath(videoPath)
	relativePath := strings.TrimPrefix(normalizedPath, "/static/videos/")
	pathParts := strings.Split(relativePath, "/")
	if len(pathParts) < 1 {
		log.Printf("警告: 无法从视频路径中提取动画标题: %s\n", videoPath)
		return
	}

	animeTitle := pathParts[0]

	movedCoverDirsMutex.Lock()
	if movedCoverDirs[animeTitle] {
		movedCoverDirsMutex.Unlock()
		return
	}
	movedCoverDirs[animeTitle] = true
	movedCoverDirsMutex.Unlock()

	videoDir := filepath.Join(videosDir, animeTitle)

	if _, err := os.Stat(videoDir); os.IsNotExist(err) {
		log.Printf("警告: 视频目录 %s 不存在，无法移动封面文件\n", videoDir)
		return
	}

	hlsDirPath := StorageServiceInstance.GetHLSPath(animeTitle)

	if err := os.MkdirAll(hlsDirPath, 0755); err != nil {
		log.Printf("警告: 创建HLS目录失败: %v\n", err)
		return
	}

	coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}
	for _, format := range coverFormats {
		coverPath := filepath.Join(videoDir, format)
		if _, err := os.Stat(coverPath); err == nil {
			targetPath := filepath.Join(hlsDirPath, format)
			if err := os.Rename(coverPath, targetPath); err != nil {
				log.Printf("警告: 移动封面文件失败: %v，尝试复制方式\n", err)
				if data, readErr := ioutil.ReadFile(coverPath); readErr == nil {
					if writeErr := ioutil.WriteFile(targetPath, data, 0644); writeErr == nil {
						if deleteErr := os.Remove(coverPath); deleteErr == nil {
							log.Printf("成功: 封面文件已复制到HLS目录: %s\n", targetPath)
						} else {
							log.Printf("警告: 复制封面文件成功，但删除原文件失败: %v\n", deleteErr)
						}
					} else {
						log.Printf("警告: 复制封面文件失败: %v\n", writeErr)
					}
				} else {
					log.Printf("警告: 读取封面文件失败: %v\n", readErr)
				}
			} else {
				log.Printf("成功: 封面文件已移动到HLS目录: %s\n", targetPath)
			}
			break
		}
	}
}

func (s *VideoService) DeleteAnime(folderName string) error {
	originalDir := filepath.Join(videosDir, folderName)
	if err := os.RemoveAll(originalDir); err != nil {
		log.Printf("警告: 删除原始视频目录失败: %v\n", err)
	} else {
		log.Printf("成功删除原始视频目录: %s\n", originalDir)
	}

	hlsDirPath := filepath.Join(hlsDir, folderName)
	if err := os.RemoveAll(hlsDirPath); err != nil {
		log.Printf("警告: 删除HLS目录失败: %v\n", err)
	} else {
		log.Printf("成功删除HLS目录: %s\n", hlsDirPath)
	}

	if !LocalMode && DB != nil {
		result := DB.Where("folder_name = ?", folderName).Delete(&models.AnimeInfo{})
		if result.Error != nil {
			log.Printf("错误: 从数据库删除动画信息失败: %v\n", result.Error)
		} else {
			log.Printf("成功从数据库删除动画信息: %s\n", folderName)
		}

		result = DB.Where("video_url LIKE ?", "%"+folderName+"%").Delete(&models.PlayHistory{})
		if result.Error != nil {
			log.Printf("错误: 从数据库删除播放记录失败: %v\n", result.Error)
		} else {
			log.Printf("成功从数据库删除相关播放记录: %s\n", folderName)
		}
	}

	return nil
}

func (s *VideoService) ExtractAnimeDirectory(videoPath string) string {
	normalizedPath := utils.NormalizeURLPath(videoPath)
	relativePath := strings.TrimPrefix(normalizedPath, "/static/")
	pathParts := strings.Split(relativePath, "/")

	if len(pathParts) >= 2 {
		if pathParts[0] == "videos" {
			return pathParts[1]
		}
		if pathParts[0] == "hls" && len(pathParts) >= 2 {
			return pathParts[1]
		}
	}

	return ""
}

func (s *VideoService) GetHLSURL(videoPath string) string {
	return s.getHLSURL(videoPath)
}

func (s *VideoService) UpdateAnimeInfo(anime models.AnimeInfo) {
	s.updateAnimeInfo(anime)
}

func (s *VideoService) ScanAnimeDirectories(directories []string) []models.AnimeInfo {
	var animes []models.AnimeInfo
	var mutex sync.Mutex
	var wg sync.WaitGroup

	dirMap := make(map[string]bool)
	uniqueDirs := []string{}
	for _, dir := range directories {
		if dir != "" && !dirMap[dir] {
			dirMap[dir] = true
			uniqueDirs = append(uniqueDirs, dir)
		}
	}

	log.Printf("开始增量扫描 %d 个动画目录...", len(uniqueDirs))

	for _, dirName := range uniqueDirs {
		hlsAnimePath := filepath.Join(hlsDir, dirName)

		if _, err := os.Stat(hlsAnimePath); os.IsNotExist(err) {
			log.Printf("警告: 目录 %s 不存在，跳过扫描", hlsAnimePath)
			continue
		}

		wg.Add(1)
		go func(name string, hlsFolder string) {
			defer wg.Done()

			var videos []models.VideoFile
			hlsEntries, err := ioutil.ReadDir(hlsFolder)
			if err == nil {
				for _, hlsEntry := range hlsEntries {
					if hlsEntry.IsDir() {
						playlistPath := filepath.Join(hlsFolder, hlsEntry.Name(), "playlist.m3u8")
						if _, err := os.Stat(playlistPath); err == nil {
							hlsURL := utils.NormalizeURLPath(strings.Join([]string{"/hls", name, hlsEntry.Name(), "playlist.m3u8"}, "/"))
							videos = append(videos, models.VideoFile{
								Path:     hlsURL,
								FileName: hlsEntry.Name(),
							})
						}
					}
				}
			}

			if len(videos) > 0 {
				sort.Slice(videos, func(i, j int) bool {
					return videos[i].FileName < videos[j].FileName
				})

				mainVideo := videos[0]
				coverURL := "/static/css/default-cover.jpg"
				coverFormats := []string{"cover.jpg", "cover.png", "cover.jpeg", "cover.webp"}

				for _, format := range coverFormats {
					hlsCoverPath := filepath.Join(hlsDir, name, format)
					if _, err := os.Stat(hlsCoverPath); err == nil {
						coverURL = utils.NormalizeURLPath(strings.Join([]string{"/hls", name, format}, "/"))
						break
					}
				}

				anime := models.AnimeInfo{
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

				if !LocalMode {
					s.updateAnimeInfo(anime)
				}
			}
		}(dirName, hlsAnimePath)
	}

	wg.Wait()

	sort.Slice(animes, func(i, j int) bool {
		return animes[i].Title < animes[j].Title
	})

	log.Printf("增量扫描完成，更新了 %d 个动画信息", len(animes))

	return animes
}
