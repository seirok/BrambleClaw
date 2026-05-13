package logger

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// AnalyzeLogs reads specified log file, gets last n lines, and formats output to console
func AnalyzeLogs(logPath string, n int) error {
	L().Debug().Str("log_path", logPath).Int("lines", n).Msg("Starting to read and analyze logs")

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
