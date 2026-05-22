package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// LogLevel maps zerolog level strings to integers for comparison
var logLevelRank = map[string]int{
	"trace": 0,
	"debug": 1,
	"info":  2,
	"warn":  3,
	"error": 4,
	"fatal": 5,
	"panic": 6,
}

// shouldShow returns true if the log line's level is >= minLevel
func shouldShow(lineLevel, minLevel string) bool {
	a, okA := logLevelRank[strings.ToLower(lineLevel)]
	b, okB := logLevelRank[strings.ToLower(minLevel)]
	if !okA || !okB {
		return true
	}
	return a >= b
}

// AnalyzeLogs reads specified log file, gets last n lines, and formats output to console
func AnalyzeLogs(logPath string, n int, minLevel string) error {
	L().Debug().Str("log_path", logPath).Int("lines", n).Str("min_level", minLevel).Msg("Starting to read and analyze logs")

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log file does not exist(%s): %w", logPath, err)
		}
		return fmt.Errorf("failed to open log file(%s): %w", logPath, err)
	}
	defer file.Close()

	// Optimization: use circular queue to keep last n lines, avoid loading entire file into memory
	lines := make([]string, 0, n)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		if len(lines) < n {
			lines = append(lines, text)
		} else {
			// array full, discard first line (can be optimized, but simple implementation is fast enough)
			lines = append(lines[1:], text)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read log file content(%s): %w", logPath, err)
	}

	// 创建一个控制台输出器，用于格式化输出
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02 15:04:05",
		NoColor:    false,
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse level from JSON for filtering
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			if level, ok := entry["level"].(string); ok && !shouldShow(level, minLevel) {
				continue
			}
		}

		// 将 JSON 字符串写入 consoleWriter 进行格式化
		// 注意 ConsoleWriter.Write 期望每行一个 JSON
		_, err := consoleWriter.Write([]byte(line + "\n"))
		if err != nil {
			// 如果格式化失败，用日志记录原始内容
			L().Warn().Str("line", line).Msg("Failed to format log line")
		}
	}

	return nil
}

// ClearLog truncates the log file, or creates it if it doesn't exist
func ClearLog(logPath string) error {
	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to clear log file(%s): %w", logPath, err)
	}
	f.Close()
	return nil
}
