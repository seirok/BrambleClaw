package cli

import (
	"brambleclaw/config"
	"brambleclaw/logger"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// AgentCreator Agent 创建向导
type AgentCreator struct {
	scanner *bufio.Scanner
	cfg     *config.Config
	cfgPath string
}

// NewAgentCreator 创建 Agent 创建向导
func NewAgentCreator(cfg *config.Config, cfgPath string) *AgentCreator {
	return &AgentCreator{
		scanner: bufio.NewScanner(os.Stdin),
		cfg:     cfg,
		cfgPath: cfgPath,
	}
}

// Run 运行创建向导
func (c *AgentCreator) Run() error {
	fmt.Println("============================================")
	fmt.Println("        创建新 Agent 向导")
	fmt.Println("============================================")
	fmt.Println()

	// 1. 输入 Agent 名称
	name, err := c.promptName()
	if err != nil {
		return err
	}

	// 2. 输入描述
	description := c.promptString("请输入 Agent 描述 (可选): ", "")

	// 3. 配置 LLM
	fmt.Println()
	fmt.Println("--- LLM 配置 ---")
	llmConfig, err := c.promptLLMConfig()
	if err != nil {
		return err
	}

	// 4. 验证 LLM 配置
	fmt.Println()
	fmt.Println("正在验证 LLM 配置...")
	if err := c.validateLLM(llmConfig); err != nil {
		return err
	}

	// 5. 选择工具
	fmt.Println()
	tools := c.promptTools()

	// 6. 配置工作目录
	fmt.Println()
	workspace := c.promptWorkspace(name)

	// 7. 配置最大历史消息数
	maxHistory := c.promptInt("最大历史消息数 (默认 5): ", 5)

	// 8. 确认创建
	fmt.Println()
	fmt.Println("--- 配置摘要 ---")
	fmt.Printf("名称: %s\n", name)
	fmt.Printf("描述: %s\n", description)
	fmt.Printf("模型: %s\n", llmConfig.Model)
	fmt.Printf("工具: %v\n", tools)
	fmt.Printf("工作目录: %s\n", workspace)
	fmt.Printf("最大历史: %d\n", maxHistory)
	fmt.Println()

	if !c.promptConfirm("确认创建此 Agent?") {
		fmt.Println("已取消创建")
		return nil
	}

	// 9. 创建 Agent 配置
	agentConfig := config.AgentConfig{
		Name:        name,
		Description: description,
		LLM:         llmConfig,
		Tools:       tools,
		Workspace:   workspace,
		MaxHistory:  maxHistory,
		Enabled:     true,
	}

	// 10. 添加到配置
	c.cfg.Agents = append(c.cfg.Agents, agentConfig)

	// 11. 保存配置
	if err := c.saveConfig(); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}

	fmt.Println()
	fmt.Println("============================================")
	fmt.Printf("Agent '%s' 创建成功!\n", name)
	fmt.Println("============================================")

	return nil
}

// promptName 提示输入 Agent 名称
func (c *AgentCreator) promptName() (string, error) {
	for {
		name := c.promptString("请输入 Agent 名称: ", "")
		if name == "" {
			fmt.Println("名称不能为空，请重新输入")
			continue
		}

		// 检查是否已存在
		for _, agent := range c.cfg.Agents {
			if agent.Name == name {
				fmt.Printf("Agent '%s' 已存在，请使用其他名称\n", name)
				continue
			}
		}

		return name, nil
	}
}

// promptLLMConfig 提示输入 LLM 配置
func (c *AgentCreator) promptLLMConfig() (config.LLMConfig, error) {
	// 如果有全局配置，使用全局配置作为默认值
	defaultConfig := c.cfg.LLMConfig

	fmt.Printf("请输入 API Key (默认: %s): ", maskAPIKey(defaultConfig.APIKey))
	apiKey := c.readLine()
	if apiKey == "" {
		apiKey = defaultConfig.APIKey
	}

	fmt.Printf("请输入 Base URL (默认: %s): ", defaultConfig.BaseURL)
	baseURL := c.readLine()
	if baseURL == "" {
		baseURL = defaultConfig.BaseURL
	}

	fmt.Printf("请输入模型名称 (默认: %s): ", defaultConfig.Model)
	model := c.readLine()
	if model == "" {
		model = defaultConfig.Model
	}

	return config.LLMConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	}, nil
}

// validateLLM 验证 LLM 配置
func (c *AgentCreator) validateLLM(llmConfig config.LLMConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := config.ValidateLLMConfig(ctx, llmConfig)
	if err != nil {
		return fmt.Errorf("验证失败: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("验证失败: %s", result.Message)
	}

	fmt.Printf("✓ 验证成功! 延迟: %v, 使用 token: %d\n", result.Latency, result.TokenUsed)
	return nil
}

// promptTools 提示选择工具
func (c *AgentCreator) promptTools() []string {
	availableTools := []string{
		"filesystem",
		"shell",
		"code_sandbox",
		"web_search",
		"url_parse",
	}

	fmt.Println("可用工具:")
	for i, tool := range availableTools {
		fmt.Printf("  %d. %s\n", i+1, tool)
	}
	fmt.Println("  0. 完成选择")

	var selected []string
	for {
		input := c.promptString("请输入工具编号 (0 完成): ", "")
		if input == "0" {
			break
		}

		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(availableTools) {
			fmt.Println("无效的选择，请重新输入")
			continue
		}

		tool := availableTools[num-1]
		// 检查是否已选择
		alreadySelected := false
		for _, t := range selected {
			if t == tool {
				alreadySelected = true
				break
			}
		}
		if alreadySelected {
			fmt.Printf("'%s' 已选择\n", tool)
			continue
		}

		selected = append(selected, tool)
		fmt.Printf("已添加: %s\n", tool)
	}

	return selected
}

// promptWorkspace 提示输入工作目录
func (c *AgentCreator) promptWorkspace(agentName string) string {
	defaultWorkspace := filepath.Join("workspace", agentName)
	fmt.Printf("请输入工作目录 (默认: %s): ", defaultWorkspace)
	workspace := c.readLine()
	if workspace == "" {
		workspace = defaultWorkspace
	}

	// 确保目录存在
	if err := os.MkdirAll(workspace, 0755); err != nil {
		logger.L().Warn().Err(err).Str("path", workspace).Msg("创建工作目录失败")
	}

	return workspace
}

// saveConfig 保存配置到文件
func (c *AgentCreator) saveConfig() error {
	data, err := json.MarshalIndent(c.cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(c.cfgPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败(%s): %w", c.cfgPath, err)
	}

	return nil
}

// promptString 提示输入字符串
func (c *AgentCreator) promptString(prompt, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s (默认: %s): ", prompt, defaultValue)
	} else {
		fmt.Print(prompt)
	}

	value := c.readLine()
	if value == "" {
		return defaultValue
	}
	return value
}

// promptInt 提示输入整数
func (c *AgentCreator) promptInt(prompt string, defaultValue int) int {
	fmt.Printf("%s (默认: %d): ", prompt, defaultValue)
	input := c.readLine()
	if input == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(input)
	if err != nil {
		fmt.Println("输入无效，使用默认值")
		return defaultValue
	}
	return value
}

// promptConfirm 提示确认
func (c *AgentCreator) promptConfirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	input := strings.ToLower(strings.TrimSpace(c.readLine()))
	return input == "y" || input == "yes"
}

// readLine 读取一行输入
func (c *AgentCreator) readLine() string {
	c.scanner.Scan()
	return strings.TrimSpace(c.scanner.Text())
}

// maskAPIKey 遮罩 API Key
func maskAPIKey(apiKey string) string {
	if apiKey == "" {
		return "(空)"
	}
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}
