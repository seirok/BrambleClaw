package runtime

import (
	"context"
	"sync"

	"neoclaw/internal/agent"
	"neoclaw/internal/logger"
	"neoclaw/internal/messages"
)

type ChatAgentContainer struct {
	agent       agent.ChatAgent
	runtime     *AgentRuntime
	inputTopic  TopicID
	outputTopic TopicID
	cancel      context.CancelFunc
	mu          sync.Mutex
	started     bool
}

func NewChatAgentContainer(
	ag agent.ChatAgent,
	rt *AgentRuntime,
	inputTopic TopicID,
	outputTopic TopicID,
) *ChatAgentContainer {
	return &ChatAgentContainer{
		agent:       ag,
		runtime:     rt,
		inputTopic:  inputTopic,
		outputTopic: outputTopic,
	}
}

func (c *ChatAgentContainer) Agent() agent.ChatAgent { return c.agent }

func (c *ChatAgentContainer) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return nil
	}

	containerCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.started = true

	sub := c.runtime.SubscribeTo(containerCtx, c.inputTopic)

	go func() {
		for {
			select {
			case <-containerCtx.Done():
				return
			case msg, ok := <-sub.Ch():
				if !ok {
					return
				}
				chatMsg, ok := msg.(messages.ChatMessage)
				if !ok {
					continue
				}
				c.processMessage(containerCtx, chatMsg)
			}
		}
	}()

	return nil
}

func (c *ChatAgentContainer) processMessage(ctx context.Context, msg messages.ChatMessage) {
	resp, err := c.agent.OnMessages(ctx, []messages.ChatMessage{msg})
	if err != nil {
		logger.L().Error().Err(err).Str("agent", c.agent.Name()).Msg("Agent.OnMessages failed in container")
		errMsg := messages.NewAgentErrorMessage(c.agent.Name(), err.Error()).WithIsProgramError(true)
		c.runtime.PublishTo(ctx, c.outputTopic, errMsg)
		return
	}

	if resp == nil || resp.ChatMessage == nil {
		return
	}

	for _, inner := range resp.InnerMessages {
		c.runtime.PublishTo(ctx, c.outputTopic, inner)
	}

	c.runtime.PublishTo(ctx, c.outputTopic, resp.ChatMessage)

	if messages.IsHandoffMessage(resp.ChatMessage) {
		target := messages.GetHandoffTarget(resp.ChatMessage)
		if target != "" {
			targetTopic := TopicID("agent:" + target)
			c.runtime.PublishTo(ctx, targetTopic, resp.ChatMessage)
		}
	}
}

func (c *ChatAgentContainer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return
	}
	c.cancel()
	c.started = false
}
