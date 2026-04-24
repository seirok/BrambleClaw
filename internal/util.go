package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func BuildSessionKey(agentName, channelName, chatID string) string {
	sessionKey := channelName + "::" + agentName + "::" + chatID
	return sessionKey
}

func ParseSessionKey(sessionKey string) (agentName, channelName, chatID string, err error) {
	// 按 "::" 进行切割
	parts := strings.Split(sessionKey, "::")

	// 严谨起见，先检查长度是否符合预期（应该有 3 部分）
	if len(parts) == 3 {
		channelName = parts[0]
		agentName = parts[1]
		chatID = parts[2]
	} else {
		return "", "", "", fmt.Errorf("invalid session key")
	}

	return agentName, channelName, chatID, nil
}

func PrintStack() {
	fmt.Println("--- Current Call Stack ---")
	// 创建一个缓冲区来存放堆栈信息
	buf := make([]byte, 1024)
	n := runtime.Stack(buf, false) // false 表示只获取当前 goroutine 的堆栈

	fmt.Printf("%s\n", buf[:n])
}

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
