package interfaces

import (
	"context"
)

// Manager 是一个通用的资源管理器接口
type Manager[T Service] interface {
	// Initialize 初始化管理器及其管理的资源
	Initialize(ctx context.Context, cfg any) error

	// StartAll 启动管理器（处理异步任务、监听等）
	StartAll(ctx context.Context) error

	// StopAll 停止管理器并回收资源
	StopAll(ctx context.Context) error

	// Add 添加一个资源
	Add(ctx context.Context, id string, item T) error

	// Remove 移除一个资源
	Remove(ctx context.Context, id string) error

	// Get 获取一个资源
	Get(ctx context.Context, id string) (T, error)

	// List 返回所有受管资源的列表
	List(ctx context.Context) []T

	// Status 返回管理器的当前运行状态
	Status() ManagerStatus
}

type Registry[V any] interface {
	// Register 注册一个条目。如果已存在，根据实现决定是覆盖还是返回 ErrAlreadyExists
	Register(ctx context.Context, name string, value V) error

	// Unregister 注销一个条目
	Unregister(ctx context.Context, name string) error

	// Get 获取条目，返回 error 以便区分“未找到”和其他系统故障
	Get(ctx context.Context, name string) (V, error)

	List(ctx context.Context) []V
}

type Message interface {
	// 获取消息内容
	Body() []byte
}

type Service interface {
	// Start 启动服务，ctx 用于控制启动过程的超时（例如初始化数据库连接）
	Start(ctx context.Context) error

	// Stop 停止服务，ctx 用于控制优雅退出的时限
	Stop(ctx context.Context) error

	Name() string
}

// Command 是 Agent 命令接口
type Command interface {
	Name() string
	Description() string
	Usage() string
	Execute(ctx context.Context, agent interface{}, msg interface{}, args []string) error
}

type Storage[T any] interface {
	Save(ctx context.Context, key string, data *T) error
	Load(ctx context.Context, key string) (*T, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

type Builder interface {
	Build() (string, error)
}
