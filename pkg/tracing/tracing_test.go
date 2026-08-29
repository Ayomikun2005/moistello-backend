package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/moistello/backend/pkg/tracing"
)

// spanAttr returns the value of the named attribute on the span, failing the
// test if it is absent.
func spanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string) attribute.Value {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value
		}
	}
	t.Fatalf("span %q is missing attribute %q", span.Name(), key)
	return attribute.Value{}
}

// TestChildSpans_FormSpanTree asserts that the DB, Redis and Stellar span
// helpers produce a proper parent → children span tree with the required
// operation/table attributes and a recorded duration (#223).
func TestChildSpans_FormSpanTree(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	// Root span stands in for the HTTP request span created by otelgin.
	rootCtx, root := tp.Tracer("test").Start(context.Background(), "GET /v1/circles")

	_, dbSpan := tracing.StartDBSpan(rootCtx, "SELECT", "circles")
	tracing.EndSpan(dbSpan, nil, time.Now())

	_, redisSpan := tracing.StartRedisSpan(rootCtx, "rate_limit.check")
	tracing.EndSpan(redisSpan, nil, time.Now())

	_, stellarSpan := tracing.StartStellarSpan(rootCtx, "get_account")
	tracing.EndSpan(stellarSpan, nil, time.Now())

	root.End()

	spans := recorder.Ended()
	require.Len(t, spans, 4)

	byName := make(map[string]sdktrace.ReadOnlySpan, len(spans))
	for _, s := range spans {
		byName[s.Name()] = s
	}

	db, ok := byName["db.SELECT"]
	require.True(t, ok, "expected a db.SELECT span")
	require.Equal(t, root.SpanContext().SpanID(), db.Parent().SpanID(), "db span should be a child of the root span")
	require.Equal(t, "postgresql", spanAttr(t, db, "db.system").AsString())
	require.Equal(t, "SELECT", spanAttr(t, db, "db.operation").AsString())
	require.Equal(t, "circles", spanAttr(t, db, "db.table").AsString())
	require.True(t, spanAttr(t, db, "duration.ms").AsInt64() >= 0, "duration should be recorded")

	redis, ok := byName["redis.rate_limit.check"]
	require.True(t, ok, "expected a redis.rate_limit.check span")
	require.Equal(t, root.SpanContext().SpanID(), redis.Parent().SpanID())
	require.Equal(t, "redis", spanAttr(t, redis, "db.system").AsString())

	stellar, ok := byName["stellar.get_account"]
	require.True(t, ok, "expected a stellar.get_account span")
	require.Equal(t, root.SpanContext().SpanID(), stellar.Parent().SpanID())
	require.Equal(t, "stellar", spanAttr(t, stellar, "rpc.system").AsString())
}

// TestEndSpan_RecordsError marks a span errored when the wrapped operation
// fails, so latency tracing also surfaces failures.
func TestEndSpan_RecordsError(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	_, span := tracing.StartDBSpan(context.Background(), "UPDATE", "users")
	tracing.EndSpan(span, context.DeadlineExceeded, time.Now())

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	require.Equal(t, sdktrace.Status{Code: codes.Error, Description: context.DeadlineExceeded.Error()}, ended[0].Status())
}
