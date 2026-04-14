package agent

import (
	"brambleclaw/config"
	"brambleclaw/logger"
	"brambleclaw/tools"
	"context"
	"testing"
)

func TestOrchestrator(t *testing.T) {
	shellTool := tools.NewShellTool()
	fileTool := tools.NewFileSystemTool()
	toolRegistry := tools.NewToolRegistry()
	toolRegistry.Register(shellTool)
	toolRegistry.Register(fileTool)

	config, _ := config.Load("../config/config.json")
	llmClient := NewLLMClient(config.LLMConfig)

	orchestrator := NewOrchestrator(llmClient, toolRegistry)
	reply, err := orchestrator.Run(context.Background(), []AgentMessage{
		AgentMessage{
			Role: RoleUser,
			Content: []ContentBlock{
				TextContent{
					Text: "从当前目录下随机选择一个文件，把文件名给我",
				},
			},
		},
	})
	if err != nil {
		logger.L().Error().Msg(err.Error())
		return
	}
	logger.L().Info().Msg(reply)

}
