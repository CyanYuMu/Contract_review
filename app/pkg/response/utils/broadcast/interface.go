package broadcast

import (
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/broadcast/adapter"
)

type Broadcast interface {
	/*Init
	 * @Description:
	 * @param cnf  RedisBroadcastConf | UdpBroadcastConf
	 * @return error
	 */
	Init(ctx context.Context, cnf adapter.Conf) error
	Subscribe(ctx context.Context) error
	Publish(ctx context.Context, msg []byte) error
	RegisterHandler(handler func(ctx context.Context, msg []byte))
}

func NewRedisBroadcast() Broadcast {
	return &adapter.RedisBroadcast{}
}

func NewUdpBroadcast() Broadcast {
	return &adapter.UdpBroadcast{}
}
