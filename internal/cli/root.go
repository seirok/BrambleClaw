package cli

import (
	"fmt"
	util "neoclaw/internal"
	"neoclaw/internal/config"
	"neoclaw/internal/logger"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "neoclaw",
	Short: "Go-based AI Agent framework",
	Long:  `neoclaw is a Go language implementation of an AI Agent framework.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Output and format recent logs and analyze session",
	RunE:  runDebug,
}

var (
	debugLines   int
	debugSession bool
)

func init() {
	rootCmd.AddCommand(versionCmd)

	debugCmd.Flags().IntVarP(&debugLines, "lines", "n", 100, "Number of recent log lines to output")
	debugCmd.Flags().BoolVarP(&debugSession, "session", "s", false, "Analyze session")
	rootCmd.AddCommand(debugCmd)
}

func Execute() error {
	config.Init()
	logger.L().Debug().Msg("Configuration loaded successfully")

	err := rootCmd.Execute()
	if err != nil {
		logger.L().Error().Err(err).Msg("Command execution failed")
	}
	return err
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println("neoclaw 1.0.0")
	fmt.Println("License: MIT")
}

func runDebug(cmd *cobra.Command, args []string) error {
	if debugSession {
		return runDebugSessions()
	}

	logPath := util.GetLogPath()
	logger.L().Debug().Str("log_path", logPath).Int("lines", debugLines).Msg("Starting formatted log output")

	err := logger.AnalyzeLogs(logPath, debugLines)
	if err != nil {
		return err
	}
	return nil
}

// runDebugSessions runs session analyzer
func runDebugSessions() error {
	fmt.Println("============================================")
	fmt.Println("        Session Analyzer")
	fmt.Println("============================================")
	fmt.Println()

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("configuration not initialized")
	}

	logger.Setup(cfg.Log.Level, cfg.Log.ConsoleEnabled)

	return nil
}
