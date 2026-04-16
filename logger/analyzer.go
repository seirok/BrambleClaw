package logger

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
)

// AnalyzeLogs 读取指定日志文件，获取最近 n 行，并格式化输出到控制台
func AnalyzeLogs(logPath string, n int) error {
	L().Debug().Str("log_path", logPath).Int("lines", n).Msg("开始读取和分析日志")

	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("日志文件不存在(%s): %w", logPath, err)
		}
		return fmt.Errorf("打开日志文件失败(%s): %w", logPath, err)
	}
	defer file.Close()

	// 优化：使用循环队列保存最近的 n 行，避免将大文件全量读入内存
	lines := make([]string, 0, n)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		if len(lines) < n {
			lines = append(lines, text)
		} else {
			// 数组已满，丢弃第一行（可以优化，但简单实现足够快）
			lines = append(lines[1:], text)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("读取日志文件内容失败(%s): %w", logPath, err)
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
			// 如果格式化失败，直接打印原始内容
			fmt.Println(line)
		}
	}

	return nil
}
