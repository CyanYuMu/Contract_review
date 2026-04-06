package server

import (
	"github.com/ansrivas/fiberprometheus/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

func PrometheusHandle(fiber *fiber.App, collectors []prometheus.Collector, name string, stage string, route string) error {
	su_logger.Info(nil, "promethus start")
	// prometheusMiddleware := fiberprometheus.NewWithRegistry(registry, config.Get().App.Name, config.Get().App.Stage, "SA", map[string]string{})
	// prometheusMiddleware.RegisterAt(di.App, "/metrics")
	prometheus.MustRegister(collectors...)
	pro := fiberprometheus.NewWithRegistry(prometheus.NewRegistry(), name, stage, name, map[string]string{})

	pro.RegisterAt(fiber, route)
	su_logger.Info(nil, "prometheus register at "+route)
	return nil
}
