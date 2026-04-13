package cli

import (
	"bufio"
	"context"
	"fmt"
	"miniGoClaw/bus"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "goclaw",
	Short: "Go-based AI Agent framework",
	Long:  `goclaw is a Go language implementation of an AI Agent framework.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run:   runVersion,
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "TUI",
	Run:   runTui,
}
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the goclaw agent",
	Run:   runStart,
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(startCmd)
	// 添加其他命令...
}

func runTui(cmd *cobra.Command, args []string) {
	scanner := bufio.NewScanner(os.Stdin)
	ctx, _ := context.WithCancel(context.Background())
	fmt.Println("goclaw CLI - Enter your message (Ctrl+C to exit):")
	msgBus := bus.NewMessageBus(100)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			input := scanner.Text()
			if input == "" {
				continue
			}

			inboundMsg := &bus.InBoundMessage{
				InChannel: "userChannel",
				SenderID:  "cli",
				ChatID:    "default",
				Content:   input,
				TimeStamp: time.Now(),
			}

			// 发布到消息总线
			msgBus.PublishInBoundMessage(ctx, inboundMsg)
		}
	}
}
func Execute() error {
	return rootCmd.Execute()
}

func runVersion(cmd *cobra.Command, args []string) {
	fmt.Println("goclaw 1.0.0")
	fmt.Println("License: MIT")
}

func runStart(cmd *cobra.Command, args []string) {
	fmt.Println("Starting goclaw agent...")
	// 启动逻辑...

}
