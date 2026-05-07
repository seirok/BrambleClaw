package util

import (
	"brambleclaw/internal/interfaces"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func BuildSessionKey(agentName, channelName, chatID string) string {
	sessionKey := channelName + "::" + agentName + "::" + chatID
	return sessionKey
}

func ParseSessionKey(sessionKey string) (agentName, channelName, chatID string, err error) {
	// 按 "_" 进行切割
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

func SessionKeyToFile(sessionKey string) string {
	agentName, channelName, chatID, err := ParseSessionKey(sessionKey)
	if err != nil {
		return ""
	}
	return agentName + "_" + channelName + "_" + chatID
}

func GetSessionFile(sessionKey string) string {
	return sessionKey + interfaces.SessionSuffix
}

func GetSessionMetaFile(sessionKey string) string {
	return sessionKey + interfaces.MetaSuffix
}

func CheckEnviromentVirable() bool {
	envs := []string{"BRAMBLE_KEY", "BRAMBLE_URL", "BRAMBLE_MODEL"}
	missing := false
	for _, env := range envs {
		if os.Getenv(env) == "" {
			// NOTE: Cannot use logger.L() here due to circular import (logger -> util -> logger)
			fmt.Printf("[ERROR] Environment variable %s is not set.\n", env)
			missing = true
		}
	}
	if missing {
		fmt.Println("Please set the required environment variables to connect to the LLM.")
		return false
	}

	return true
}

func SaveStructToJSON(filePath string, data any) error {
	// 1. 将结构体序列化为 JSON 字节流
	// 使用 MarshalIndent 增加缩进，让生成的 JSON 文件具有可读性
	bytes, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal struct: %w", err)
	}

	// 2. 自动创建父级目录（如果不存在）
	// 例如路径是 ~/.brambleclaw/settings.json，它会先创建 .brambleclaw 文件夹
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// 3. 将字节流写入文件
	// 0644 表示：所有者可读写，组和其他人只读
	err = os.WriteFile(filePath, bytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func LoadJSONToStruct(filePath string, data any) error {
	// 1. 读取文件内容
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// 2. 将 JSON 字节流解析到目标结构体
	// data 这里必须是指针，否则 Unmarshal 无法修改其内容
	err = json.Unmarshal(bytes, data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal json from %s: %w", filePath, err)
	}

	return nil
}

func MakeItHome(path string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, path)
}

func GetGlobalConfigPath() string {
	return MakeItHome(filepath.Join(".brambleclaw", "settings.json"))
}

func GetLogPath() string {
	return MakeItHome(filepath.Join(".brambleclaw", "logs", "bramble.log"))
}

func GetSystemPath() string {
	return MakeItHome(".brambleclaw")
}

func StringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// ExtractJSONStringField unmarshals content as JSON and returns the value of the given string field.
// Returns "" if the content is invalid JSON or the field is missing/empty.
func ExtractJSONStringField(content, field string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return ""
	}
	raw, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}
