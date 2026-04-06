package limiter

import (
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type Controller interface {
	Handle(ctx context.Context, source string, err error)
}

type LoggerController struct {
}

func (l *LoggerController) Handle(ctx context.Context, source string, err error) {
	su_logger.Warn(ctx, "flow blocked source:"+source, su_logger.E().Error(err))
}

type AlertController struct {
}

func (a *AlertController) Handle(ctx context.Context, source string, err error) {
	su_logger.WarnWithNotify(ctx, "flow blocked source:"+source, su_logger.E().Error(err))
}

func NewController(t LimiterController) Controller {
	switch t {
	case ControllerLogger:
		return &LoggerController{}
	case ControllerAlert:
		return &AlertController{}
	default:
		return nil
	}
}
