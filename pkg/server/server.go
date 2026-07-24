package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/observerbility"
	"github.com/heptiolabs/healthcheck"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type HttpServer struct {
	listener net.Listener
	server   *http.Server
	mux      *http.ServeMux
	health   healthcheck.Handler
}

// NewHttpServer creates a new http server instance which includes tracing, healthchecks and shutdown using signals and
// contexts.
func NewHttpServer(port uint16) (*HttpServer, error) {
	mux := http.NewServeMux()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	server := &http.Server{
		Handler:  mux,
		ErrorLog: log.Default(),
		// Without an IdleTimeout, idle keep-alive connections are kept open
		// forever, and each one retains its per-connection serve/connReader
		// goroutines. A client (browser, scraper, probe) that keeps opening new
		// connections faster than it reuses them would grow the goroutine count
		// without bound until the goroutine-threshold healthcheck trips. Reaping
		// idle connections bounds goroutines to actually-active connections.
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &HttpServer{
		listener: listener,
		server:   server,
		health:   healthcheck.NewHandler(),
		mux:      mux,
	}, nil
}

// AddHandleFunc registers the given handlerFunc for the given pattern.
func (s *HttpServer) AddHandleFunc(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
	s.AddHandler(pattern, http.HandlerFunc(handlerFunc))
}

// AddHandler registers the given handler for the given pattern.
func (s *HttpServer) AddHandler(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, otelhttp.NewHandler(handler, pattern))
}

// AddLivenessCheck registers the given healthcheck as liveness check.
func (s *HttpServer) AddLivenessCheck(name string, check healthcheck.Check) {
	s.health.AddLivenessCheck(name, check)
}

// AddReadinessCheck registers the given healthcheck as readiness check.
func (s *HttpServer) AddReadinessCheck(name string, check healthcheck.Check) {
	s.health.AddReadinessCheck(name, check)
}

func (s *HttpServer) Addr() net.Addr {
	return s.listener.Addr()
}

// Run starts the http server and blocks until it either receives SIGINT or the given context ends.
func (s *HttpServer) Run(ctx context.Context) error {
	// Handle SIGINT (CTRL+C) gracefully.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	// Set up OpenTelemetry.
	otelShutdown, err := observerbility.SetupOTelSDK(ctx)
	if err != nil {
		return err
	}
	// Handle shutdown properly so nothing leaks.
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()

	s.server.BaseContext = func(_ net.Listener) context.Context { return ctx }

	s.mux.HandleFunc("/live", s.health.LiveEndpoint)
	s.mux.HandleFunc("/ready", s.health.ReadyEndpoint)

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- s.server.Serve(s.listener)

	}()

	// Wait for interruption.
	select {
	case err = <-srvErr:
		// Error when starting HTTP server.
		return err
	case <-ctx.Done():
		// Wait for first CTRL+C.
		// Stop receiving signal notifications as soon as possible.
		stop()
	}

	// When Shutdown is called, ListenAndServe immediately returns ErrServerClosed.
	err = s.server.Shutdown(context.Background())
	return err
}
