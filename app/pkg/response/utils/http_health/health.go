package http_health

import (
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/config"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"
)

func PprofHandle(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

func IsPprofOn(v string) bool {
	pprofFlag := strings.ToLower(v)
	if pprofFlag == "" || pprofFlag == "off" || pprofFlag == "false" {
		return false
	}
	return true
}

func LaunchHttpHealthCheckServer(ctf interface{}) *http.Server {
	c, o := ctf.(config.App)
	if !o {
		panic("ctf except config.App")
	}
	addr := c.Addr
	if c.Port == "" {
		panic("请配置http端口")
	}
	addr += ":" + c.Port
	server := http.Server{
		Addr:        addr,
		ReadTimeout: 6 * time.Second,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/system/health", ok)
	server.Handler = mux
	if IsPprofOn(c.Pprof) {
		PprofHandle(mux)
		su_logger.Info(nil, "pprof enabled")
	} else {
		su_logger.Info(nil, "pprof disabled")
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()

	return &server
}

func ok(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(("ok")))
}
