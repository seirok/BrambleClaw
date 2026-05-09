package cli

import (
	util "brambleclaw/internal"
	"brambleclaw/internal/config"
	"brambleclaw/internal/logger"
	"fmt"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "brambleclaw",
	Short: "Go-based AI Agent framework",
	Long:  `brambleclaw is a Go language implementation of an AI Agent framework.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "输出并格式化最近的日志和分析 session",
	RunE:  runDebug,
}

var (
	debugLines   int
	debugSession bool
)

func init() {
	rootCmd.AddCommand(versionCmd)

	debugCmd.Flags().IntVarP(&debugLines, "lines", "n", 100, "输出最近的日志行数")
	debugCmd.Flags().BoolVarP(&debugSession, "session", "s", false, "分析 session")
	rootCmd.AddCommand(debugCmd)
}

func Execute() error {
	config.Init()
	logger.L().Debug().Msg("配置加载成功")

	err := rootCmd.Execute()
	if err != nil {
		logger.L().Error().Err(err).Msg("命令执行失败")
	}
	return err
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println("brambleclaw 1.0.0")
	fmt.Println("License: MIT")
}

func runDebug(cmd *cobra.Command, args []string) error {
	if debugSession {
		return runDebugSessions()
	}

	logPath := util.GetLogPath()
	logger.L().Debug().Str("log_path", logPath).Int("lines", debugLines).Msg("开始格式化输出日志")

	err := logger.AnalyzeLogs(logPath, debugLines)
	if err != nil {
		return err
	}
	return nil
}

// runDebugSessions 运行 session 分析器
func runDebugSessions() error {
	fmt.Println("============================================")
	fmt.Println("        Session 分析器")
	fmt.Println("============================================")
	fmt.Println()

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("配置未初始化")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)

	return nil
}
