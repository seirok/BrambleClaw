package store

import (
	"brambleclaw/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileStorage 是 Storage 接口的文件系统实现
type FileStorage[T any] struct {
	DataDir string
	mu      sync.RWMutex // 使用读写锁，Save 方法使用写锁，Load/Exists 使用读锁
}

// NewFileStorage 创建一个新的 FileStorage 实例
func NewFileStorage[T any](dataDir string) *FileStorage[T] {
	return &FileStorage[T]{
		DataDir: dataDir,
	}
}

// Save 将数据序列化为 JSON 并原子性地保存到文件中
func (f *FileStorage[T]) Save(ctx context.Context, key string, data *T) error {
	// 使用写锁保护文件写入操作
	f.mu.Lock()
	defer f.mu.Unlock()

	path := filepath.Join(f.DataDir, key)

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", filepath.Dir(path), err)
	}

	// 序列化数据
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal data: %w", err)
	}

	// 原子性写入：写入临时文件，然后重命名
	// 生成一个唯一的临时文件名
	tmpFileName := fmt.Sprintf("%s.%s.%d", path, "tmp", time.Now().UnixNano())

	// 写入临时文件
	if err := os.WriteFile(tmpFileName, buf, 0644); err != nil {
		return fmt.Errorf("could not write to temporary file %s: %w", tmpFileName, err)
	}

	// 确保临时文件在函数返回前被清理，无论成功或失败
	defer func() {
		if r := recover(); r != nil { // 处理可能的 panic
			os.Remove(tmpFileName)
			logger.L().Error().Interface("panic", r).Str("path", path).Msg("Panic during file save")
			panic(r)
		}
		if err != nil { // 如果 Save 方法返回错误，清理临时文件
			os.Remove(tmpFileName)
		}
	}()

	// 重命名临时文件为目标文件，此操作在大多数文件系统上是原子性的
	if err := os.Rename(tmpFileName, path); err != nil {
		// 如果重命名失败，尝试清理临时文件
		os.Remove(tmpFileName)
		return fmt.Errorf("could not rename temporary file %s to %s: %w", tmpFileName, path, err)
	}

	return nil
}

// Load 从文件中读取数据并反序列化为指定类型
func (f *FileStorage[T]) Load(ctx context.Context, key string) (*T, error) {
	// 使用读锁保护文件读取操作
	f.mu.RLock()
	defer f.mu.RUnlock()

	path := filepath.Join(f.DataDir, key)

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	// 读取文件内容
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read file %s: %w", path, err)
	}

	// 反序列化数据
	var data T
	if err := json.Unmarshal(buf, &data); err != nil {
		return nil, fmt.Errorf("could not unmarshal data from file %s: %w", path, err)
	}

	return &data, nil
}

// Delete 删除指定的文件
func (f *FileStorage[T]) Delete(ctx context.Context, key string) error {
	// 使用写锁保护文件删除操作
	f.mu.Lock()
	defer f.mu.Unlock()

	path := filepath.Join(f.DataDir, key)

	// 检查文件是否存在，如果不存在则直接返回 nil
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // 文件不存在，视为删除成功
	}

	// 删除文件
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("could not delete file %s: %w", path, err)
	}

	return nil
}

// Exists 检查文件是否存在
func (f *FileStorage[T]) Exists(ctx context.Context, key string) (bool, error) {
	// 使用读锁保护文件存在性检查操作
	f.mu.RLock()
	defer f.mu.RUnlock()

	path := filepath.Join(f.DataDir, key)

	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("could not check file existence for %s: %w", path, err)
}
