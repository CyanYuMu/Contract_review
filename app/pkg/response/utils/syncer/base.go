package syncer

import (
	"context"
)

type Synchronizer interface {
	Publish(ctx context.Context, topic string, msg string) error
	Subscribe(ctx context.Context, channels string) (<-chan string, error)
}
