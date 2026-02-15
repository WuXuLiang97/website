package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// HLS修复结果结构体
type HLSFixResult struct {
	AnimeTitle string `json:"animeTitle"`
	Episode    string `json:"episode"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
}

// 工作项结构体
type workItem struct {
	animeTitle   string
	episodeTitle string
	playlistPath string
}

// 修复HLS视频（将TS切片合成为MP4并标准化）
func fixHLSVideos(progressChan chan<- map[string]interface{}, stopChan <-chan struct{}) ([]HLSFixResult, error) {
	var results []HLSFixResult
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// 并发限制设置
	const maxConcurrency = 4 // 最多同时处理4个视频

	hlsDir := "static/hls"

	// 检查HLS目录是否存在
	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		return results, fmt.Errorf("HLS目录 %s 不存在", hlsDir)
	}

	// 扫描HLS目录中的动画文件夹
	animeDirs, err := ioutil.ReadDir(hlsDir)
	if err != nil {
		return results, fmt.Errorf("扫描HLS目录失败: %v", err)
	}

	// 收集所有工作项
	var workItems []workItem
	for _, animeDir := range animeDirs {
		if !animeDir.IsDir() {
			continue
		}

		animeTitle := animeDir.Name()
		animePath := filepath.Join(hlsDir, animeTitle)

		// 扫描动画目录中的集数文件夹
		episodeDirs, err := ioutil.ReadDir(animePath)
		if err != nil {
			mutex.Lock()
			results = append(results, HLSFixResult{
				AnimeTitle: animeTitle,
				Episode:    "",
				Success:    false,
				Message:    fmt.Sprintf("扫描集数目录失败: %v", err),
			})
			mutex.Unlock()
			continue
		}

		for _, episodeDir := range episodeDirs {
			if !episodeDir.IsDir() {
				continue
			}

			episodeTitle := episodeDir.Name()
			episodePath := filepath.Join(animePath, episodeTitle)
			playlistPath := filepath.Join(episodePath, "playlist.m3u8")

			// 检查playlist.m3u8文件是否存在
			if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
				mutex.Lock()
				results = append(results, HLSFixResult{
					AnimeTitle: animeTitle,
					Episode:    episodeTitle,
					Success:    false,
					Message:    "playlist.m3u8文件不存在",
				})
				mutex.Unlock()
				continue
			}

			// 添加工作项
			workItems = append(workItems, workItem{
				animeTitle:   animeTitle,
				episodeTitle: episodeTitle,
				playlistPath: playlistPath,
			})
		}
	}

	totalItems := len(workItems)
	processedItems := 0
	var processedMutex sync.Mutex

	// 如果没有工作项，直接返回
	if totalItems == 0 {
		// 发送完成消息
		select {
		case progressChan <- map[string]interface{}{
			"type":      "complete",
			"total":     len(results),
			"success":   countSuccessResults(results),
			"failed":    len(results) - countSuccessResults(results),
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}
		return results, nil
	}

	// 创建工作通道
	workChan := make(chan workItem, totalItems)

	// 启动工作池
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					// 停止信号，退出工作协程
					return
				case item, ok := <-workChan:
					if !ok {
						// 工作通道已关闭，退出工作协程
						return
					}

					// 处理工作项
					processWorkItem(item, &results, &mutex, progressChan, stopChan, &processedItems, &processedMutex, totalItems)
				}
			}
		}()
	}

	// 发送工作项到工作通道
	for _, item := range workItems {
		select {
		case <-stopChan:
			// 停止信号，关闭工作通道并等待所有工作协程完成
			close(workChan)
			wg.Wait()
			return results, fmt.Errorf("处理已停止")
		case workChan <- item:
			// 工作项发送成功
		}
	}

	// 关闭工作通道
	close(workChan)

	// 等待所有工作协程完成
	wg.Wait()

	// 发送完成消息
	select {
	case progressChan <- map[string]interface{}{
		"type":      "complete",
		"total":     len(results),
		"success":   countSuccessResults(results),
		"failed":    len(results) - countSuccessResults(results),
		"timestamp": time.Now().Format(time.RFC3339),
	}:
		// 消息发送成功
	default:
		// 通道已关闭，忽略
	}

	return results, nil
}

// 获取视频帧率函数
func getVideoFramerate(videoPath string) (string, error) {
	// 使用ffprobe获取视频帧率
	cmd := exec.Command(
		"ffprobe",
		"-v", "quiet",
		"-select_streams", "v:0",
		"-show_entries", "stream=avg_frame_rate", // 使用平均帧率
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "30", fmt.Errorf("获取帧率失败: %v, 输出: %s", err, string(output))
	}

	// 处理输出，去除换行符
	framerate := strings.TrimSpace(string(output))
	if framerate == "" || framerate == "0/0" {
		return "30", fmt.Errorf("获取帧率为空或无效")
	}

	// 解析分数格式的帧率
	parts := strings.Split(framerate, "/")
	if len(parts) == 2 {
		// 尝试转换为浮点数
		numerator, err1 := strconv.Atoi(parts[0])
		denominator, err2 := strconv.Atoi(parts[1])
		if err1 == nil && err2 == nil && denominator > 0 {
			fps := float64(numerator) / float64(denominator)
			// 四舍五入到整数
			return strconv.Itoa(int(math.Round(fps))), nil
		}
	}

	// 如果解析失败，返回默认帧率
	return "30", nil
}

// 处理单个工作项
func processWorkItem(item workItem, results *[]HLSFixResult, mutex *sync.Mutex, progressChan chan<- map[string]interface{}, stopChan <-chan struct{}, processedItems *int, processedMutex *sync.Mutex, totalItems int) {
	// 检查是否需要停止
	select {
	case <-stopChan:
		return
	default:
		// 继续处理
	}

	// 发送进度更新
	select {
	case progressChan <- map[string]interface{}{
		"type":      "progress",
		"anime":     item.animeTitle,
		"episode":   item.episodeTitle,
		"status":    "processing",
		"timestamp": time.Now().Format(time.RFC3339),
	}:
		// 消息发送成功
	default:
		// 通道已关闭，忽略
	}

	// 获取原视频帧率
	framerate, err := getVideoFramerate(item.playlistPath)
	if err != nil {
		// 发送警告更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "warning",
			"anime":     item.animeTitle,
			"episode":   item.episodeTitle,
			"message":   fmt.Sprintf("获取帧率失败，使用默认30fps: %v", err),
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}
		framerate = "30" // 使用默认帧率
	} else {
		// 发送信息更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "info",
			"anime":     item.animeTitle,
			"episode":   item.episodeTitle,
			"message":   fmt.Sprintf("获取原视频帧率: %s", framerate),
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}
	}

	// 构建输出文件路径
	outputDir := filepath.Join("static/fixed_videos", item.animeTitle, item.episodeTitle)
	playlistPath := filepath.Join(outputDir, "playlist.m3u8")
	segmentPath := filepath.Join(outputDir, "segment_%03d.ts")

	// 确保输出目录存在
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		mutex.Lock()
		*results = append(*results, HLSFixResult{
			AnimeTitle: item.animeTitle,
			Episode:    item.episodeTitle,
			Success:    false,
			Message:    fmt.Sprintf("创建输出目录失败: %v", err),
		})
		mutex.Unlock()

		// 发送错误更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "error",
			"anime":     item.animeTitle,
			"episode":   item.episodeTitle,
			"message":   fmt.Sprintf("创建输出目录失败: %v", err),
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}

		// 更新处理计数
		processedMutex.Lock()
		*processedItems++
		currentCount := *processedItems
		processedMutex.Unlock()

		// 发送总体进度更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "overall_progress",
			"current":   currentCount,
			"total":     totalItems,
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}

		return
	}

	// 构建FFmpeg命令（高级标准化修复，输出HLS格式）
	cmd := exec.Command(
		"ffmpeg",
		// 【可选】m3u8网络播放列表专用，本地文件可删除
		"-protocol_whitelist", "file,http,https,tcp,tls",
		"-allowed_extensions", "ALL",
		// 输入文件
		"-i", item.playlistPath,
		// 【核心修复】剥离错误封装信息，提取原始帧
		"-fflags", "+genpts+igndts+discardcorrupt", // 生成新时间戳+忽略DTS+丢弃损坏帧
		"-err_detect", "aggressive", // 激进的错误检测
		"-bsf:a", "aac_adtstoasc", // 音频ADTS头转换
		// 【核心修复】音视频同步与时间戳重建
		"-fps_mode", "cfr", // 使用固定帧率模式
		"-vsync", "1", // 视频同步方式：1=CFR
		"-async", "1", // 音频同步方式：1=自动
		"-shortest",                       // 以最短流为准
		"-avoid_negative_ts", "make_zero", // 避免负时间戳
		"-reset_timestamps", "1", // 重置时间戳
		// 【核心修复】重新编码确保完整性
		"-c:v", "h264_nvenc", // NVIDIA专用H.264硬件编码器
		"-preset", "p7", // 编码预设：p1最快，p7画质最好
		"-crf", "28", // 码率控制：数值越小画质越好
		"-tune", "hq", // 调优：hq=画质优先
		"-profile:v", "high", // 编码配置：high
		"-level", "4.1", // 编码级别：4.1
		// 音频编码
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2",
		"-ar", "48000", // 音频采样率
		// HLS输出参数（标准HLS格式）
		"-hls_time", "3", // 切片时间，单位秒
		"-hls_list_size", "0", // 保留所有切片
		"-hls_segment_filename", segmentPath, // 切片文件路径模板
		"-hls_flags", "split_by_time+independent_segments", // 按时间分割+独立切片
		"-hls_allow_cache", "1", // 允许缓存
		"-hls_segment_type", "mpegts", // 切片类型
		"-hls_base_url", "./", // 基础URL
		// 覆盖文件
		"-y",
		playlistPath,
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 构建详细的错误信息
		errorMsg := fmt.Sprintf("执行FFmpeg命令失败: %v, 命令: %v, 输出: %s", err, cmd.Args, string(output))

		mutex.Lock()
		*results = append(*results, HLSFixResult{
			AnimeTitle: item.animeTitle,
			Episode:    item.episodeTitle,
			Success:    false,
			Message:    errorMsg,
		})
		mutex.Unlock()

		// 发送错误更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "error",
			"anime":     item.animeTitle,
			"episode":   item.episodeTitle,
			"message":   errorMsg,
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}

		// 更新处理计数
		processedMutex.Lock()
		*processedItems++
		currentCount := *processedItems
		processedMutex.Unlock()

		// 发送总体进度更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "overall_progress",
			"current":   currentCount,
			"total":     totalItems,
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}

		return
	}

	// 修复成功
	mutex.Lock()
	*results = append(*results, HLSFixResult{
		AnimeTitle: item.animeTitle,
		Episode:    item.episodeTitle,
		Success:    true,
		Message:    fmt.Sprintf("修复成功，输出HLS播放列表: %s", playlistPath),
	})
	mutex.Unlock()

	// 删除原始HLS文件
	originalHLSDir := filepath.Dir(item.playlistPath)
	if err := os.RemoveAll(originalHLSDir); err != nil {
		// 发送删除错误更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "warning",
			"anime":     item.animeTitle,
			"episode":   item.episodeTitle,
			"message":   fmt.Sprintf("删除原始HLS文件失败: %v", err),
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}
	} else {
		// 发送删除成功更新
		select {
		case progressChan <- map[string]interface{}{
			"type":      "info",
			"anime":     item.animeTitle,
			"episode":   item.episodeTitle,
			"message":   "删除原始HLS文件成功",
			"timestamp": time.Now().Format(time.RFC3339),
		}:
			// 消息发送成功
		default:
			// 通道已关闭，忽略
		}
	}

	// 发送成功更新
	select {
	case progressChan <- map[string]interface{}{
		"type":      "success",
		"anime":     item.animeTitle,
		"episode":   item.episodeTitle,
		"output":    playlistPath,
		"timestamp": time.Now().Format(time.RFC3339),
	}:
		// 消息发送成功
	default:
		// 通道已关闭，忽略
	}

	// 更新处理计数
	processedMutex.Lock()
	*processedItems++
	currentCount := *processedItems
	processedMutex.Unlock()

	// 发送总体进度更新
	select {
	case progressChan <- map[string]interface{}{
		"type":      "overall_progress",
		"current":   currentCount,
		"total":     totalItems,
		"timestamp": time.Now().Format(time.RFC3339),
	}:
		// 消息发送成功
	default:
		// 通道已关闭，忽略
	}
}

// 统计成功的修复结果数量
func countSuccessResults(results []HLSFixResult) int {
	count := 0
	for _, result := range results {
		if result.Success {
			count++
		}
	}
	return count
}

// 修复HLS视频的API处理函数
func fixHLSVideosHandler(c *gin.Context) {
	// 设置响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 创建进度通道和停止通道
	progressChan := make(chan map[string]interface{}, 100) // 带缓冲的通道
	stopChan := make(chan struct{})

	// 启动修复处理
	go func() {
		defer close(progressChan) // 确保通道关闭
		defer close(stopChan)     // 确保停止通道关闭

		_, err := fixHLSVideos(progressChan, stopChan)
		if err != nil {
			// 发送错误消息
			select {
			case progressChan <- map[string]interface{}{
				"type":      "error",
				"message":   fmt.Sprintf("修复过程失败: %v", err),
				"timestamp": time.Now().Format(time.RFC3339),
			}:
				// 消息发送成功
			default:
				// 通道已关闭，忽略
			}
		}
	}()

	// 发送进度更新
	for {
		select {
		case progress, ok := <-progressChan:
			if !ok {
				// 通道已关闭，退出循环
				return
			}
			data, err := json.Marshal(progress)
			if err != nil {
				continue
			}
			_, err = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			if err != nil {
				// 客户端连接已关闭，退出循环
				return
			}
			c.Writer.Flush() // 刷新缓冲区
		case <-time.After(30 * time.Second):
			// 发送心跳消息，保持连接活跃
			_, err := fmt.Fprintf(c.Writer, "data: {\"type\":\"heartbeat\",\"timestamp\":\"%s\"}\n\n", time.Now().Format(time.RFC3339))
			if err != nil {
				// 客户端连接已关闭，退出循环
				return
			}
			c.Writer.Flush() // 刷新缓冲区
		}
	}
}
