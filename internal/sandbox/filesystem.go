package sandbox

import (
	"brambleclaw/internal/logger"
	"context"
	"os"
)

// Helper 函数 - 导出供 tools 包使用

func ReadFileWithContext(ctx context.Context, path string) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		data, err := readFileLimited(path, 10*1024*1024) // 10MB limit
		ch <- result{data, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.data, res.err
	}
}

func WriteFileWithContext(ctx context.Context, path string, data []byte, perm uint32) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)
	go func() {
		err := writeFileLimited(path, data, perm, 10*1024*1024) // 10MB limit
		ch <- result{err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.err
	}
}

func AppendFileWithContext(ctx context.Context, path string, data []byte, perm uint32) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)
	go func() {
		var err error
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.FileMode(perm))
		if err != nil {
			ch <- result{err}
			return
		}
		defer file.Close()
		_, err = file.Write(data)
		ch <- result{err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.err
	}
}

func EnsureDir(path string) error {
	return ensureDir(path)
}

func ReadDirWithContext(ctx context.Context, path string) ([]os.DirEntry, error) {
	type result struct {
		entries []os.DirEntry
		err     error
	}

	ch := make(chan result, 1)
	go func() {
		entries, err := os.ReadDir(path)
		ch <- result{entries, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.entries, res.err
	}
}

func DeleteWithContext(ctx context.Context, path string) error {
	type result struct {
		err error
	}

	ch := make(chan result, 1)
	go func() {
		err := os.RemoveAll(path)
		ch <- result{err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		return res.err
	}
}

// 带大小限制的文件读取
func readFileLimited(path string, maxSize int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.Size() > maxSize {
		return nil, os.ErrInvalid // 使用标准错误
	}

	return os.ReadFile(path)
}

// 带大小限制的文件写入
func writeFileLimited(path string, data []byte, perm uint32, maxSize int64) error {
	logger.L().Debug().Str("path", path).Msg("writeFileLimited: begin to write limited file")
	if int64(len(data)) > maxSize {
		return os.ErrInvalid // 使用标准错误
	}

	return os.WriteFile(path, data, os.FileMode(perm))
}
