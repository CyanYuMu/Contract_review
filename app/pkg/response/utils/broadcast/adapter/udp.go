package adapter

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-redis/redis"
	"github.com/spf13/cast"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/service_resolver"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"net"
	"sync"
)

type UdpBroadcast struct {
	BroadcastBase
	serverName  string
	ip          string
	port        int
	serviceKey  string
	resolver    *service_resolver.ServiceResolver
	connHandler map[string]*net.UDPConn
	lock        sync.Mutex
}

type UdpBroadcastConf struct {
	Ip          string
	ServiceName string
	Port        int
	Redis       redis.UniversalClient
}

func (u *UdpBroadcast) Init(ctx context.Context, c Conf) error {
	cnf := c.Udp
	u.serverName = cnf.ServiceName
	u.ip = cnf.Ip

	r := service_resolver.New()
	err := r.Init(ctx, service_resolver.ServiceResolverConf{
		Redis:       cnf.Redis,
		Port:        cnf.Port,
		ServiceName: u.serverName,
	})
	if err != nil {
		return errors.New("init service resolver failed " + err.Error())
	}

	if cnf.Port < 1000 {
		return fmt.Errorf("upd port must be greater than 1000")
	}

	u.port = cnf.Port
	u.resolver = r
	u.serviceKey = getKey(u.ip, cnf.Port)
	u.connHandler = make(map[string]*net.UDPConn, 1)

	return nil
}

func getKey(ip string, port int) string {
	return fmt.Sprintf("%s_%d", ip, port)
}

func (u *UdpBroadcast) Subscribe(ctx context.Context) error {
	lis, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: cast.ToInt(u.port),
	})
	if err != nil {
		return err
	}
	defer lis.Close()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var data [1024]byte
			// 接收数据
			n, _, err := lis.ReadFromUDP(data[:])
			if err != nil {
				su_logger.Error(ctx, err, "read from udp failed")
				continue
			}
			if n == 0 {
				continue
			}
			// 通知到所有句柄
			u.notify(ctx, data[:n])
		}
	}
}

func (u *UdpBroadcast) Publish(ctx context.Context, msg []byte) error {
	// 排除自身
	u.notify(ctx, msg)

	for _, addr := range u.resolver.Resolver() {
		//  获取节点列表并发送消息
		key := getKey(addr.Ip, addr.Port)
		if key == u.serviceKey {
			continue
		}
		if _, ok := u.connHandler[key]; !ok {
			u.lock.Lock()
			if _, ok = u.connHandler[key]; !ok {
				ip := net.ParseIP(addr.Ip).To4()
				conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
					IP:   ip,
					Port: addr.Port,
					Zone: "",
				})
				if err != nil {
					u.lock.Unlock()
					continue
				}
				u.connHandler[key] = conn
				u.lock.Unlock()
			}
		}

		_, err := u.connHandler[key].Write(msg)
		if err != nil {
			// 从连接句柄中删除
			u.connHandler[key].Close()
			delete(u.connHandler, key)
			su_logger.Error(ctx, err, "write msg", su_logger.E().String("msg", string(msg)))
		}
	}

	return nil
}
