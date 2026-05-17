package cron

import (
	"context"
	"neoclaw/internal/bus"
	"neoclaw/internal/logger"
	"time"

	"github.com/google/uuid"
)

// GenerateJobID 生成唯一任务ID
func GenerateJobID() string {
	return uuid.New().String()
}

// CalculateNextRunAt 计算下次执行时间
func CalculateNextRunAt(schedule CronSchedule, nowMS int64) int64 {
	if schedule.Kind == "Every" {
		return nowMS + schedule.EveryMS
	}
	return schedule.AtMS
}

// DeliverDirect 直投路径:直接向channel发送消息
func DeliverDirect(ctx context.Context, msgBus *bus.MessageBus, job *CronJob) error {
	outMsg := &bus.OutBoundMessage{
		ChatID:     job.Payload.To,
		OutChannel: job.Payload.Channel,
		Content:    job.Payload.Message,
		MsgType:    "text",
		ReplyTo:    "",
		TimeStamp:  time.Now(),
	}
	if err := msgBus.PublishOutBoundMessage(ctx, outMsg); err != nil {
		logger.L().Error().Err(err).Str("job", job.ID).Msg("Failed to deliver cron message directly")
		return err
	}
	logger.L().Debug().Str("job", job.ID).Str("channel", job.Payload.Channel).Msg("Delivered cron message directly")
	return nil
}

// DeliverToAgent Agent驱动路径:注入gateway让daemon处理
func DeliverToAgent(ctx context.Context, msgBus *bus.MessageBus, job *CronJob) error {
	inMsg := &bus.InBoundMessage{
		ID:        GenerateJobID(),
		SenderID:  "cron:" + job.ID,
		ChatID:    "cron:" + job.ID,
		InChannel: "cron",
		Content:   job.Payload.Message,
		TimeStamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	if job.Payload.ReplyToChannel != "" {
		inMsg.Metadata["reply_channel"] = job.Payload.ReplyToChannel
	}
	if job.Payload.ReplyToChat != "" {
		inMsg.Metadata["reply_chat"] = job.Payload.ReplyToChat
	}

	if err := msgBus.PublishInBoundMessage(ctx, inMsg); err != nil {
		logger.L().Error().Err(err).Str("job", job.ID).Msg("Failed to deliver cron message to agent")
		return err
	}
	logger.L().Debug().Str("job", job.ID).Msg("Delivered cron message to daemon agent")
	return nil
}
