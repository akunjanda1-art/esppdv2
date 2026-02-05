package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func NewHTTPMetrics(service string) *HTTPMetrics {
	reqs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "esppd",
		Subsystem:   "http",
		Name:        "requests_total",
		Help:        "Total HTTP requests",
		ConstLabels: prometheus.Labels{"service": service},
	}, []string{"method", "route", "status"})

	dur := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   "esppd",
		Subsystem:   "http",
		Name:        "request_duration_seconds",
		Help:        "HTTP request duration in seconds",
		ConstLabels: prometheus.Labels{"service": service},
		Buckets:     []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	}, []string{"method", "route", "status"})

	prometheus.MustRegister(reqs, dur)

	return &HTTPMetrics{requests: reqs, duration: dur}
}

func (m *HTTPMetrics) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		route := ""
		if r := c.Route(); r != nil {
			route = r.Path
		}
		if route == "" {
			route = c.Path()
		}

		status := strconv.Itoa(c.Response().StatusCode())
		m.requests.WithLabelValues(c.Method(), route, status).Inc()
		m.duration.WithLabelValues(c.Method(), route, status).Observe(time.Since(start).Seconds())

		return err
	}
}

func Handler() fiber.Handler {
	return adaptor.HTTPHandler(promhttp.Handler())
}
