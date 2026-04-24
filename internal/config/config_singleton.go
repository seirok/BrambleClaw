package config

import (
	"sync"
)

var (
	// globalConfig 全局配置单例实例
	globalConfig *Config
	// globalPath 实际加载的配置文件路径
	globalPath string
	// once 确保单例只被初始化一次
	once sync.Once
	// initErr 记录初始化过程中的错误
	initErr error
)

// Init 初始化全局配置单例
// 应在程序启动时调用一次（如 main.go 中）
// path: 显式配置文件路径，为空字符串时使用自动搜索
func Init(path string) error {
	once.Do(func() {
		loader := NewLoader()
		if path != "" {
			loader.ExplicitPath = path
		}
		globalConfig, globalPath, initErr = loader.Load()
	})
	return initErr
}

// MustInit 初始化全局配置单例，失败时 panic
// 适用于配置必须存在的场景
func MustInit(path string) {
	if err := Init(path); err != nil {
		panic("配置初始化失败: " + err.Error())
	}
}

// Get 获取全局配置单例实例
// 注意：在调用 Init 之前调用此方法会返回 nil
func Get() *Config {
	return globalConfig
}

// GetPath 获取实际加载的配置文件路径
func GetPath() string {
	return globalPath
}

// IsInitialized 检查配置是否已初始化
func IsInitialized() bool {
	return globalConfig != nil
}

// Reload 重新加载配置（用于配置热更新场景）
// 注意：此方法会重置 sync.Once，需要并发安全控制
func Reload(path string) (*Config, error) {
	loader := NewLoader()
	if path != "" {
		loader.ExplicitPath = path
	}

	cfg, loadedPath, err := loader.Load()
	if err != nil {
		return nil, err
	}

	// 原子性更新（简单赋值在 Go 中是指针原子操作）
	globalConfig = cfg
	globalPath = loadedPath

	return cfg, nil
}
