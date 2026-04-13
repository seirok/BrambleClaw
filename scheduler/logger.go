package scheduler

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

type DefaultLogger struct {
	mu       sync.Mutex
	level    LogLevel
	prefix   string
	logFile  *os.File
}

func NewDefaultLogger() *DefaultLogger {
	return &DefaultLogger{
		level:  LogLevelInfo,
		prefix: "[scheduler] ",
	}
}

func NewLoggerWithFile(filePath string, level LogLevel) (*DefaultLogger, error) {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	return &DefaultLogger{
		level:   level,
		prefix:  "[scheduler] ",
		logFile: file,
	}, nil
}

func (l *DefaultLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *DefaultLogger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
}

func (l *DefaultLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

func (l *DefaultLogger) Debug(msg string, args ...any) {
	l.log(LogLevelDebug, "DEBUG", msg, args...)
}

func (l *DefaultLogger) Info(msg string, args ...any) {
	l.log(LogLevelInfo, "INFO", msg, args...)
}

func (l *DefaultLogger) Warn(msg string, args ...any) {
	l.log(LogLevelWarn, "WARN", msg, args...)
}

func (l *DefaultLogger) Error(msg string, args ...any) {
	l.log(LogLevelError, "ERROR", msg, args...)
}

func (l *DefaultLogger) log(level LogLevel, levelStr, msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	formattedMsg := fmt.Sprintf(msg, args...)
	logMsg := fmt.Sprintf("%s%s [%s] %s\n", l.prefix, timestamp, levelStr, formattedMsg)

	if l.logFile != nil {
		l.logFile.WriteString(logMsg)
	} else {
		log.Print(logMsg)
	}
}

type NoOpLogger struct{}

func NewNoOpLogger() *NoOpLogger {
	return &NoOpLogger{}
}

func (l *NoOpLogger) Debug(msg string, args ...any) {}

func (l *NoOpLogger) Info(msg string, args ...any) {}

func (l *NoOpLogger) Warn(msg string, args ...any) {}

func (l *NoOpLogger) Error(msg string, args ...any) {}

type MultiLogger struct {
	loggers []Logger
}

func NewMultiLogger(loggers ...Logger) *MultiLogger {
	return &MultiLogger{
		loggers: loggers,
	}
}

func (l *MultiLogger) AddLogger(logger Logger) {
	l.loggers = append(l.loggers, logger)
}

func (l *MultiLogger) Debug(msg string, args ...any) {
	for _, logger := range l.loggers {
		logger.Debug(msg, args...)
	}
}

func (l *MultiLogger) Info(msg string, args ...any) {
	for _, logger := range l.loggers {
		logger.Info(msg, args...)
	}
}

func (l *MultiLogger) Warn(msg string, args ...any) {
	for _, logger := range l.loggers {
		logger.Warn(msg, args...)
	}
}

func (l *MultiLogger) Error(msg string, args ...any) {
	for _, logger := range l.loggers {
		logger.Error(msg, args...)
	}
}
