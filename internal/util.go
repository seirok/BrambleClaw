package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func BuildSessionKey(agentName, channelName, chatID string) string {
	sessionKey := channelName + "_" + agentName + "_" + chatID
	return sessionKey
}

func ParseSessionKey(sessionKey string) (agentName, channelName, chatID string, err error) {
	// 按 "::" 进行切割
	parts := strings.Split(sessionKey, "_")

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

// IsFirstLevelSubDir 检查 sub 是否是 parent 的直接子目录
func IsFirstLevelSubDir(parent, sub string) (bool, error) {
	// 1. 先检查输入是否符合要求
	if parent == "" || sub == "" {
		return false, fmt.Errorf("parent 和 sub 路径都不能为空")
	}

	// 2. 处理相对路径 - 如果 sub 是相对路径，则相对于 parent 解析
	var absParent, absSub string
	var err error

	if filepath.IsAbs(parent) {
		absParent = filepath.Clean(parent)
	} else {
		absParent, err = filepath.Abs(parent)
		if err != nil {
			return false, fmt.Errorf("无法获取 parent 的绝对路径: %w", err)
		}
	}

	if filepath.IsAbs(sub) {
		absSub = filepath.Clean(sub)
	} else {
		// 如果是相对路径，相对于 parent 目录解析
		absSub = filepath.Clean(filepath.Join(absParent, sub))
	}

	// 3. 检查 sub 是否是目录
	subInfo, err := os.Stat(absSub)
	if err != nil {
		// 如果路径不存在，返回 false 且不报错（避免中断测试）
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !subInfo.IsDir() {
		return false, nil // 不是目录，直接返回
	}

	// 4. 严格检查层级关系
	// - 路径必须不同 (不能是同一个目录)
	// - sub 的父路径必须等于 parent
	if absParent == absSub {
		return false, nil
	}

	subParent := filepath.Dir(absSub)
	return subParent == absParent, nil
}
