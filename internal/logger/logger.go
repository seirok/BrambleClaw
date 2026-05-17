package logger

import (
	util "neoclaw/internal"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	instance *zerolog.Logger
	mu       sync.Mutex
)

// CustomCallerHook 允许在日志中包含调用者信息
type CustomCallerHook struct{}

func (h CustomCallerHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	// 只在特定级别开启 Caller 显示，减少生产环境开销
	if level == zerolog.DebugLevel || level == zerolog.ErrorLevel || level == zerolog.FatalLevel {
		// 3 通常跳转到调用 L().Debug() 的业务代码处
		e.Caller(3)
	}
}

// Setup 根据配置初始化或重新初始化日志
func Setup(level string, consoleEnabled bool) {
	mu.Lock()
	defer mu.Unlock()

	// 1. 解析日志级别
	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		lvl = zerolog.InfoLevel
	}

	// 关键点：必须设置全局级别，否则实例级别会被全局默认的 Info 级别拦截
	zerolog.SetGlobalLevel(lvl)

	// 2. 配置滚动日志输出 (Lumberjack)
	fileWriter := &lumberjack.Logger{
		Filename:   util.GetLogPath(),
		MaxSize:    100, // megabytes
		MaxBackups: 5,
		MaxAge:     30,   // days
		Compress:   true, // disabled by default
	}

	var multiWriter zerolog.LevelWriter
	if consoleEnabled {
		// 3. 配置控制台输出 (带颜色)
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006-01-02 15:04:05",
		}
		// 4. 合并输出端
		multiWriter = zerolog.MultiLevelWriter(fileWriter, consoleWriter)
	} else {
		multiWriter = zerolog.MultiLevelWriter(fileWriter)
	}

	// 5. 创建 Logger 实例
	logger := zerolog.New(multiWriter).
		Level(lvl).
		With().
		Timestamp().
		Logger().
		Hook(CustomCallerHook{})

	instance = &logger
}

// SetLevel 运行时动态修改日志级别
func SetLevel(level string) {
	mu.Lock()
	defer mu.Unlock()

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		if instance != nil {
			instance.Error().Err(err).Msg("failed to parse log level")
		}
		return
	}

	// 同步修改全局级别
	zerolog.SetGlobalLevel(lvl)

	if instance != nil {
		// 更新实例
		newLogger := instance.Level(lvl)
		instance = &newLogger
	}
}

// L 获取单例 Logger 对象
func L() *zerolog.Logger {
	if instance == nil {
		// 默认初始化，防止空指针
		Setup("debug", false)
	}
	return instance
}
