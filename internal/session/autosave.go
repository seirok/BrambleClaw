package session

import (
	"context"
	"neoclaw/internal/logger"
	"time"
)

type SessionSave struct {
	session *Session
	stopCh  chan struct{} // 用于通知 goroutine 停止的信号通道
}

// NewSessionSave 初始化时创建好 stopCh
func NewSessionSave(sess *Session) *SessionSave {
	return &SessionSave{
		session: sess,
		stopCh:  make(chan struct{}),
	}
}

func (s *SessionSave) SetSession(sess *Session) {
	s.session = sess
}

func (s *SessionSave) Start(ctx context.Context) error {
	// 设定定时器，比如每 5 秒检查并保存一次
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		defer ticker.Stop() // 协程退出时释放定时器资源

		for {
			select {
			case <-ticker.C:
				// 定时到了，触发保存逻辑
				if s.session.Modified {
					// 注意：这里的 ctx 是 Start 传入的，如果 Start 执行完 ctx 就失效了，
					// 建议这里使用 context.Background() 或者专门的长生命周期 ctx
					if err := s.session.Save(context.Background()); err != nil {
						logger.L().Err(err).Msg("[SessionSave] fail to save session")
						continue
					}
					s.session.Modified = false
				}

			case <-s.stopCh:
				// 收到停止信号，优雅退出协程
				logger.L().Info().Msg("[SessionSave] closing session")
				return
			}
		}
	}()

	return nil
}

func (s *SessionSave) Stop(ctx context.Context) error {
	// 安全地关闭通道，通知 goroutine 退出
	select {
	case <-s.stopCh:
		// 防止重复关闭
	default:
		close(s.stopCh)
	}
	return nil
}

func (s *SessionSave) Name() string {
	return "session save"
}
