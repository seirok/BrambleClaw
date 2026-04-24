package util

import (
	"fmt"
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
