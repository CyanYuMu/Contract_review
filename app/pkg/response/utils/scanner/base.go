package scanner

import "context"

type Limiter interface {
	Wait(ctx context.Context) error
}
