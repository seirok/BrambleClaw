package util

import (
	"os"
	"path/filepath"
	"strings"
)

// IsFirstLevelSubDir 判断 sub 是否是 parent 的第一级子目录
func IsFirstLevelSubDir(parent, sub string) (bool, error) {
	// 1. 将路径转换为绝对路径并进行清理（消除 . 或 ..）
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return false, err
	}
	absSub, err := filepath.Abs(sub)
	if err != nil {
		return false, err
	}

	// 2. 获取相对路径
	// 如果 absSub 是 "C:/a/b"，absParent 是 "C:/a"，则 rel 为 "b"
	rel, err := filepath.Rel(absParent, absSub)
	if err != nil {
		return false, err
	}

	// 3. 逻辑判断：
	// - 不能是父目录本身 (rel == ".")
	// - 不能是父目录的父级 (rel 以 ".." 开头)
	// - 必须是第一层级（不能包含路径分隔符，如 "b/c" 是二级子目录）
	if rel == "." || strings.HasPrefix(rel, "..") || strings.ContainsRune(rel, os.PathSeparator) {
		return false, nil
	}

	// 4. 检查该路径在文件系统中是否真实存在且为目录
	info, err := os.Stat(absSub)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // 路径不存在
		}
		return false, err // 其他权限等错误
	}

	return info.IsDir(), nil
}
