package adapter

import "context"

type BroadcastBase struct {
	handlers []func(ctx context.Context, msg []byte)
}

/*RegisterHandler
* @Description: 注册消息处理函数
* @param handler
 */
func (b *BroadcastBase) RegisterHandler(handler func(ctx context.Context, msg []byte)) {
	b.handlers = append(b.handlers, handler)
}

/*notify
* @Description: 广播消息
* @param ctx
* @param msg
* @return error
 */
func (b *BroadcastBase) notify(ctx context.Context, msg []byte) {
	for i, _ := range b.handlers {
		b.handlers[i](ctx, msg)
	}
}

type Conf struct {
	// @description 基于udp方式进行广播, 与 redis 参数二选一
	Udp *UdpBroadcastConf
	// @description 基于redis方式进行广播, 与 udp 参数二选一
	Redis *RedisBroadcastConf
}
