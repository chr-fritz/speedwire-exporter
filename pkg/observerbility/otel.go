package observerbility

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/chr-fritz/speedwire-exporter/version"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdkTrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer
var tracerMutex = sync.RWMutex{}

// SetupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func SetupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider(ctx)
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(ctx context.Context) (*sdkTrace.TracerProvider, error) {
	traceExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, err
	}

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(version.AppName),
			semconv.ServiceVersion(version.Version),
		),
	)
	if err != nil {
		return nil, err
	}

	traceProvider := sdkTrace.NewTracerProvider(
		sdkTrace.WithBatcher(
			traceExporter,
			sdkTrace.WithBatchTimeout(5*time.Second),
		),
		sdkTrace.WithResource(r),
	)

	return traceProvider, nil
}

func GetTracer() trace.Tracer {
	tracerMutex.Lock()
	defer tracerMutex.Unlock()

	if tracer != nil {
		return tracer
	}
	tracer = otel.Tracer("speedwire-exporter")
	return tracer
}
