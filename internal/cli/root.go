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

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "View, filter, and manage logs",
	RunE:  runLog,
}

var (
	logLines int
	logDebug bool
	logInfo  bool
	logInit  bool
)

func init() {
	rootCmd.AddCommand(versionCmd)

	logCmd.Flags().IntVarP(&logLines, "lines", "n", 100, "Number of recent log lines to output")
	logCmd.Flags().BoolVarP(&logDebug, "debug", "d", false, "Show debug-level and above (default)")
	logCmd.Flags().BoolVarP(&logInfo, "info", "i", false, "Show info-level and above")
	logCmd.Flags().BoolVar(&logInit, "init", false, "Clear the current log file")
	rootCmd.AddCommand(logCmd)
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

func runLog(cmd *cobra.Command, args []string) error {
	if logInit {
		if logDebug || logInfo {
			return fmt.Errorf("--init cannot be combined with --debug or --info")
		}
		logPath := util.GetLogPath()
		logger.L().Info().Str("log_path", logPath).Msg("Clearing log file")
		if err := logger.ClearLog(logPath); err != nil {
			return err
		}
		fmt.Println("Log file cleared.")
		return nil
	}

	minLevel := "debug"
	if logInfo {
		minLevel = "info"
	}
	// logDebug is the default (shows everything), no need to check

	logPath := util.GetLogPath()
	logger.L().Debug().Str("log_path", logPath).Int("lines", logLines).Str("min_level", minLevel).Msg("Starting formatted log output")

	return logger.AnalyzeLogs(logPath, logLines, minLevel)
}

// runLogSessions runs session analyzer
func runLogSessions() error {
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
