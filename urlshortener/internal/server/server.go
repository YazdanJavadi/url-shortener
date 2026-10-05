// Package server wires the Echo HTTP server and the URL handlers. Handlers
// depend on the urlservice.Service (DI), not the database directly.
package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/yazdanjavadi/urlshort/internal/metrics"
	"github.com/yazdanjavadi/urlshort/internal/repository"
	"github.com/yazdanjavadi/urlshort/internal/urlservice"
)

// URLService is the contract the HTTP handlers depend on (DI). The concrete
// urlservice.Service satisfies it; tests inject a mock.
type URLService interface {
	Create(ctx context.Context, longURL string) (string, error)
	Resolve(ctx context.Context, code string) (string, error)
}

// Server holds the Echo engine and dependencies.
type Server struct {
	echo    *echo.Echo
	service URLService
	logger  *logrus.Logger
	baseURL string
	metrics *metrics.Metrics
}

// New constructs the Echo server with middleware and routes wired to the
// given service. Tracing (otelecho) and Prometheus metrics are enabled.
func New(svc URLService, log *logrus.Logger, baseURL string) *Server {
	m := metrics.New()

	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Recover())
	e.Use(otelecho.Middleware("urlshort")) // OpenTelemetry tracing per request
	e.Use(requestLogger(log))
	e.Use(metricsMiddleware(m))

	srv := &Server{echo: e, service: svc, logger: log, baseURL: baseURL, metrics: m}

	e.POST("/urls", srv.createShortURL)
	e.GET("/:code", srv.resolveShortURL)
	e.GET("/metrics", echo.WrapHandler(m.Handler())) // Prometheus scrape endpoint

	return srv
}

// Start blocks and serves HTTP on the given address.
func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

// createShortURL handles POST /urls {"url":"https://..."} -> 201 {"code","short","long_url"}.
func (s *Server) createShortURL(c echo.Context) error {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.Bind(&req); err != nil {
		s.logger.WithError(err).Warn("failed to bind create request")

		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	longURL := strings.TrimSpace(req.URL)
	if longURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "url is required")
	}

	if err := validateURL(longURL); err != nil {
		s.logger.WithError(err).WithField("long_url", longURL).Warn("invalid long url")

		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	code, err := s.service.Create(c.Request().Context(), longURL)
	if err != nil {
		if errors.Is(err, urlservice.ErrCodeGeneration) {
			s.logger.WithError(err).Error("code generation failed")

			return echo.NewHTTPError(http.StatusInternalServerError, "could not generate short code")
		}

		s.logger.WithError(err).Error("service create failed")

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	short := s.shortURL(c, code)
	spanAttrs(c,
		attribute.String("url.long_url", longURL),
		attribute.String("url.code", code),
	)
	s.logger.WithFields(logrus.Fields{
		"long_url": longURL,
		"code":     code,
		"short":    short,
	}).Info("created short url")

	return c.JSON(http.StatusCreated, echo.Map{
		"code":     code,
		"short":    short,
		"long_url": longURL,
	})
}

// resolveShortURL handles GET /:code -> 302 redirect to long URL (404 if unknown).
func (s *Server) resolveShortURL(c echo.Context) error {
	code := c.Param("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code is required")
	}

	longURL, err := s.service.Resolve(c.Request().Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.WithField("code", code).Info("short code not found")

			return echo.NewHTTPError(http.StatusNotFound, "short url not found")
		}

		s.logger.WithError(err).Error("service resolve failed")

		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	spanAttrs(c,
		attribute.String("url.code", code),
		attribute.String("url.long_url", longURL),
	)
	s.logger.WithFields(logrus.Fields{
		"code":     code,
		"long_url": longURL,
	}).Info("redirecting to long url")

	return c.Redirect(http.StatusFound, longURL)
}

// shortURL builds the absolute short URL for a code.
func (s *Server) shortURL(c echo.Context, code string) string {
	if s.baseURL != "" {
		return strings.TrimRight(s.baseURL, "/") + "/" + code
	}

	scheme := "http"
	if c.IsTLS() {
		scheme = "https"
	}

	return scheme + "://" + c.Request().Host + "/" + code
}

// validateURL requires an absolute http/https URL with a host.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url scheme must be http or https")
	}

	if u.Host == "" {
		return errors.New("url host is required")
	}

	return nil
}

// requestLogger logs each request via Logrus.
func requestLogger(log *logrus.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:    true,
		LogStatus: true,
		LogMethod: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log.WithFields(logrus.Fields{
				"method": v.Method,
				"uri":    v.URI,
				"status": v.Status,
			}).Info("request")

			return nil
		},
	})
}

// metricsMiddleware records request count and latency per method/route.
func metricsMiddleware(m *metrics.Metrics) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			elapsed := time.Since(start).Seconds()
			route := c.Path() // e.g. "/urls", "/:code", "/metrics"
			status := c.Response().Status
			m.RequestsTotal.WithLabelValues(c.Request().Method, route, itoa(status)).Inc()
			m.Latency.WithLabelValues(c.Request().Method, route).Observe(elapsed)

			return err
		}
	}
}

// itoa avoids importing strconv just for status conversion in metrics labels.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	neg := n < 0
	if neg {
		n = -n
	}

	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}

	if neg {
		i--
		b[i] = '-'
	}

	return string(b[i:])
}

// spanAttrs attaches attributes to the active span (if any) so traces carry
// business context like the code/long_url.
func spanAttrs(c echo.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(c.Request().Context())
	if span.SpanContext().IsValid() {
		span.SetAttributes(attrs...)
	}
}
