package messaging

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestAMQPHeaderCarrier_RoundTrip(t *testing.T) {
	// Create a tracer provider with an in-memory exporter for verification
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create a span context to propagate
	ctx := context.Background()
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(ctx, "test-operation", oteltrace.WithAttributes())
	span.End()

	// Inject into AMQP headers
	headers := make(map[string]interface{})
	InjectHeaders(ctx, headers)

	// Verify headers were set
	if len(headers) == 0 {
		t.Fatal("expected headers to be populated")
	}

	// Extract from AMQP headers
	extractedCtx := ExtractHeaders(context.Background(), headers)

	// Verify trace context was extracted
	sc := oteltrace.SpanContextFromContext(extractedCtx)
	if !sc.IsValid() {
		t.Fatal("expected valid span context after extraction")
	}

	// Verify trace ID matches
	originalSC := oteltrace.SpanContextFromContext(ctx)
	if sc.TraceID() != originalSC.TraceID() {
		t.Errorf("trace ID mismatch: got %s, want %s", sc.TraceID(), originalSC.TraceID())
	}
	if sc.SpanID() != originalSC.SpanID() {
		t.Errorf("span ID mismatch: got %s, want %s", sc.SpanID(), originalSC.SpanID())
	}
}

func TestAMQPHeaderCarrier_MissingHeaders(t *testing.T) {
	// Extract from nil headers should not panic and return original context
	ctx := context.Background()
	extractedCtx := ExtractHeaders(ctx, nil)
	if extractedCtx != ctx {
		t.Error("expected original context when headers are nil")
	}

	// Extract from empty headers should not panic and return original context
	headers := make(map[string]interface{})
	extractedCtx = ExtractHeaders(ctx, headers)
	if extractedCtx != ctx {
		t.Error("expected original context when headers are empty")
	}
}

func TestAMQPHeaderCarrier_InvalidHeaders(t *testing.T) {
	// Headers with invalid traceparent should not panic and return original context
	headers := map[string]interface{}{
		"traceparent": "invalid-header-value",
	}
	ctx := context.Background()
	extractedCtx := ExtractHeaders(ctx, headers)
	if extractedCtx != ctx {
		t.Error("expected original context when headers contain invalid traceparent")
	}

	// Headers with non-string values should be tolerated
	headers = map[string]interface{}{
		"traceparent": 12345,
	}
	extractedCtx = ExtractHeaders(ctx, headers)
	if extractedCtx != ctx {
		t.Error("expected original context when headers contain non-string values")
	}
}

func TestAMQPHeaderCarrier_StringAndByteValues(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	ctx := context.Background()
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(ctx, "test-operation")
	span.End()

	// Test with string values
	headers := make(map[string]interface{})
	InjectHeaders(ctx, headers)

	// Verify we can read both string and byte forms
	for k, v := range headers {
		switch v.(type) {
		case string:
			// OK
		case []byte:
			// OK
		default:
			t.Errorf("header %q has unexpected type %T", k, v)
		}
	}

	// Extract should work with both string and byte values
	extractedCtx := ExtractHeaders(context.Background(), headers)
	sc := oteltrace.SpanContextFromContext(extractedCtx)
	if !sc.IsValid() {
		t.Fatal("expected valid span context after extraction with string values")
	}

	// Test with byte-string values manually
	byteHeaders := make(map[string]interface{})
	for k, v := range headers {
		if s, ok := v.(string); ok {
			byteHeaders[k] = []byte(s)
		} else if b, ok := v.([]byte); ok {
			byteHeaders[k] = b
		}
	}
	extractedCtx = ExtractHeaders(context.Background(), byteHeaders)
	sc = oteltrace.SpanContextFromContext(extractedCtx)
	if !sc.IsValid() {
		t.Fatal("expected valid span context after extraction with byte values")
	}
}

func TestAMQPHeaderCarrier_InjectionWithNilHeaders(t *testing.T) {
	// Inject into nil headers should not panic
	ctx := context.Background()
	InjectHeaders(ctx, nil)
	// No assertion needed, just verifying no panic
}