package store

import "context"

type Storage[T any] interface {
	Save(ctx context.Context, key string, data *T) error
	Load(ctx context.Context, key string) (*T, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}
