package services

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"anime-website/config"
)

type StorageService struct {
	disks      []*Disk
	currentIdx int
	mu         sync.RWMutex
	strategy   string
}

type Disk struct {
	Name      string
	Path      string
	MaxSizeGB int
	UsedGB    float64
	Priority  int
	Enabled   bool
	LastCheck time.Time
}

var StorageServiceInstance = &StorageService{}

func (s *StorageService) Init() {
	cfg := config.Get()
	s.strategy = cfg.Storage.Strategy

	for _, diskCfg := range cfg.Storage.Disks {
		if diskCfg.Enabled {
			disk := &Disk{
				Name:      diskCfg.Name,
				Path:      diskCfg.Path,
				MaxSizeGB: diskCfg.MaxSizeGB,
				Priority:  diskCfg.Priority,
				Enabled:   true,
			}
			s.updateDiskUsage(disk)
			s.disks = append(s.disks, disk)
			log.Printf("存储服务: 添加磁盘 %s, 路径: %s, 最大容量: %dGB\n", disk.Name, disk.Path, disk.MaxSizeGB)
		}
	}

	log.Printf("存储服务初始化完成，共 %d 个磁盘，策略: %s\n", len(s.disks), s.strategy)
}

func (s *StorageService) updateDiskUsage(disk *Disk) {
	var size int64
	err := filepath.Walk(disk.Path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		log.Printf("警告: 计算磁盘 %s 使用率失败: %v\n", disk.Name, err)
		disk.UsedGB = 0
	} else {
		disk.UsedGB = float64(size) / (1024 * 1024 * 1024)
	}

	disk.LastCheck = time.Now()
}

func (s *StorageService) GetDiskForStorage(animeName string) *Disk {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.disks) == 0 {
		log.Printf("错误: 没有可用的存储磁盘\n")
		return nil
	}

	var disk *Disk
	switch s.strategy {
	case "round-robin":
		disk = s.roundRobinDisk()
	case "least-used":
		disk = s.leastUsedDisk()
	case "random":
		disk = s.randomDisk()
	default:
		disk = s.leastUsedDisk()
	}

	if disk != nil {
		log.Printf("存储服务: 为动画 %s 选择磁盘 %s (使用率: %.2fGB/%dGB)\n", animeName, disk.Name, disk.UsedGB, disk.MaxSizeGB)
	}

	return disk
}

func (s *StorageService) roundRobinDisk() *Disk {
	disk := s.disks[s.currentIdx]
	s.currentIdx = (s.currentIdx + 1) % len(s.disks)
	return disk
}

func (s *StorageService) leastUsedDisk() *Disk {
	var bestDisk *Disk
	minUsage := float64(100000)

	for _, disk := range s.disks {
		if disk.Enabled && disk.UsedGB < minUsage {
			minUsage = disk.UsedGB
			bestDisk = disk
		}
	}

	return bestDisk
}

func (s *StorageService) randomDisk() *Disk {
	for _, disk := range s.disks {
		if disk.Enabled {
			return disk
		}
	}
	return nil
}

func (s *StorageService) GetHLSPath(animeName string) string {
	disk := s.GetDiskForStorage(animeName)
	if disk == nil {
		return filepath.Join("static/hls", animeName)
	}
	return filepath.Join(disk.Path, animeName)
}

func (s *StorageService) GetHLSURL(animeName string) string {
	disk := s.GetDiskForStorage(animeName)
	if disk == nil {
		return "/hls/" + animeName
	}
	return "/storage/" + disk.Name + "/" + animeName
}

func (s *StorageService) FindDiskByAnimeName(animeName string) *Disk {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, disk := range s.disks {
		if !disk.Enabled {
			continue
		}
		animePath := filepath.Join(disk.Path, animeName)
		if _, err := os.Stat(animePath); err == nil {
			return disk
		}
	}
	return nil
}

func (s *StorageService) GetDiskByName(name string) *Disk {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, disk := range s.disks {
		if disk.Name == name {
			return disk
		}
	}
	return nil
}

func (s *StorageService) GetAllDisks() []*Disk {
	s.mu.RLock()
	defer s.mu.RUnlock()

	disks := make([]*Disk, len(s.disks))
	copy(disks, s.disks)
	return disks
}

func (s *StorageService) RefreshDiskUsage() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, disk := range s.disks {
		s.updateDiskUsage(disk)
		log.Printf("存储服务: 磁盘 %s 使用率 %.2fGB/%dGB\n", disk.Name, disk.UsedGB, disk.MaxSizeGB)
	}
}

func (s *StorageService) UpdateDiskUsage(disk *Disk) {
	s.updateDiskUsage(disk)
}
