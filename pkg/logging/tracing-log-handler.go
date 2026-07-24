package logging

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type tracingLogHandler struct {
	parent slog.Handler
}

func NewTracingLogHandler(parent slog.Handler) slog.Handler {
	return &tracingLogHandler{
		parent: parent,
	}
}

func (h tracingLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.parent.Enabled(ctx, level)
}

func (h tracingLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &tracingLogHandler{parent: h.parent.WithAttrs(attrs)}
}

func (h tracingLogHandler) WithGroup(name string) slog.Handler {
	return &tracingLogHandler{parent: h.parent.WithGroup(name)}
}

func (h tracingLogHandler) Handle(ctx context.Context, record slog.Record) error {
	spanContext := trace.SpanFromContext(ctx).SpanContext()
	if spanContext.IsValid() {
		record.AddAttrs(
			slog.String("trace.id", spanContext.TraceID().String()),
			slog.String("span.id", spanContext.SpanID().String()),
		)
	}
	return h.parent.Handle(ctx, record)
}
