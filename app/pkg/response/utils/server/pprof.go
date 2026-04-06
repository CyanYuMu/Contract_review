package server

import (
	"net/http"
	"net/http/pprof"

	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

func PprofHandle(addr string) error {
	su_logger.Info(nil, "pprof start", su_logger.E().String("addr", addr))
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	// 启动http服务器并监听端口
	return http.ListenAndServe(addr, mux)
}
