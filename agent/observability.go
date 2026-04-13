package agent

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/rfbigelow/go-agents/agent"

var tracer = otel.Tracer(tracerName)

// startSpan starts a new span and returns the updated context and span.
func startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := tracer.Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, span
}

// endSpan ends a span, recording an error if present.
func endSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// logArgs builds a []any for slog, appending trace correlation attributes
// if a valid span is present in the context.
func logArgs(ctx context.Context, keyvals ...any) []any {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	if sc.IsValid() {
		keyvals = append(keyvals,
			"trace_id", sc.TraceID().String(),
			"span_id", sc.SpanID().String(),
		)
	}
	return keyvals
}
