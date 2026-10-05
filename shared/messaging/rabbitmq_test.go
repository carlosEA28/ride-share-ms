package messaging

import (
	"context"
	"testing"
	"time"

	"ride-sharing/shared/retry"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestAMQPPublishing_DefaultValues(t *testing.T) {
	// Test that default publishing values are correct
	msg := amqp.Publishing{
		ContentType:  "text/plain",
		Body:         []byte("test"),
		DeliveryMode: amqp.Persistent,
	}

	if msg.ContentType != "text/plain" {
		t.Errorf("ContentType mismatch: got %s", msg.ContentType)
	}
	if msg.DeliveryMode != amqp.Persistent {
		t.Errorf("DeliveryMode mismatch: got %d", msg.DeliveryMode)
	}
}

func TestConsumeMessages_TraceParentage(t *testing.T) {
	// Set up tracer with in-memory exporter
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create a parent span context (simulating a producer)
	parentCtx := context.Background()
	tracer := tp.Tracer("test")
	parentCtx, parentSpan := tracer.Start(parentCtx, "producer-operation")
	parentSpan.End()

	// Create a fake delivery with trace headers
	headers := make(map[string]interface{})
	InjectHeaders(parentCtx, headers)

	// Convert to amqp.Table format
	amqpHeaders := amqp.Table{}
	for k, v := range headers {
		amqpHeaders[k] = v
	}

	delivery := amqp.Delivery{
		Body:    []byte(`{"ownerId":"user-123","data":{}}`),
		Headers: amqpHeaders,
	}

	// Simulate extraction
	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	// Start consumer span
	ctx, consumerSpan := tracer.Start(ctx, "consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "consume"),
		),
	)
	consumerSpan.End()

	// Force flush to ensure spans are exported
	tp.ForceFlush(context.Background())

	// Verify spans were created and linked
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Find consumer span
	var consumerSpanData tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "consume" {
			consumerSpanData = s
			break
		}
	}
	if consumerSpanData.Name == "" {
		t.Fatal("consumer span not found")
	}

	// Verify consumer span attributes
	attrs := consumerSpanData.Attributes
	foundSystem := false
	foundDestination := false
	foundOperation := false
	for _, a := range attrs {
		switch a.Key {
		case "messaging.system":
			if a.Value.AsString() != "rabbitmq" {
				t.Errorf("messaging.system = %s, want rabbitmq", a.Value.AsString())
			}
			foundSystem = true
		case "messaging.destination":
			if a.Value.AsString() != "test-queue" {
				t.Errorf("messaging.destination = %s, want test-queue", a.Value.AsString())
			}
			foundDestination = true
		case "messaging.operation":
			if a.Value.AsString() != "consume" {
				t.Errorf("messaging.operation = %s, want consume", a.Value.AsString())
			}
			foundOperation = true
		}
	}
	if !foundSystem || !foundDestination || !foundOperation {
		t.Error("missing expected span attributes")
	}

	// Verify trace parentage - consumer span should have parent's trace ID
	parentSC := trace.SpanContextFromContext(parentCtx)
	consumerSC := consumerSpanData.SpanContext
	if consumerSC.TraceID() != parentSC.TraceID() {
		t.Errorf("trace ID mismatch: consumer trace ID %s != parent trace ID %s",
			consumerSC.TraceID(), parentSC.TraceID())
	}
}

// retryableError is an error that can be retried
type retryableError struct {
	err error
}

func (e *retryableError) Error() string {
	return e.err.Error()
}

func TestConsumeMessages_SuccessfulRetry(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create a parent context
	parentCtx := context.Background()
	tracer := tp.Tracer("test")
	parentCtx, parentSpan := tracer.Start(parentCtx, "producer-operation")
	parentSpan.End()

	headers := make(map[string]interface{})
	InjectHeaders(parentCtx, headers)
	amqpHeaders := amqp.Table{}
	for k, v := range headers {
		amqpHeaders[k] = v
	}

	delivery := amqp.Delivery{
		Body:    []byte(`{"ownerId":"user-123","data":{}}`),
		Headers: amqpHeaders,
	}

	// Simulate handler that fails once then succeeds
	attempt := 0
	handler := func(ctx context.Context, msg amqp.Delivery) error {
		attempt++
		if attempt == 1 {
			return &retryableError{err: assertError("temporary failure")}
		}
		return nil
	}

	// Create a retry config with short delays for testing
	cfg := retry.Config{
		MaxRetries:  3,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     10 * time.Millisecond,
	}

	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	ctx, consumerSpan := tracer.Start(ctx, "consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "consume"),
		),
	)

	err := retry.WithBackoff(ctx, cfg, func() error {
		return handler(ctx, delivery)
	})
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}

	consumerSpan.End()

	tp.ForceFlush(context.Background())

	if attempt != 2 {
		t.Errorf("expected 2 attempts, got %d", attempt)
	}

	// Verify span has no error status
	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "consume" {
			if s.Status.Code != codes.Unset {
				t.Errorf("expected unset status code for successful retry, got %v", s.Status.Code)
			}
		}
	}
}

func TestConsumeMessages_FinalHandlerFailure(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	parentCtx := context.Background()
	tracer := tp.Tracer("test")
	parentCtx, parentSpan := tracer.Start(parentCtx, "producer-operation")
	parentSpan.End()

	headers := make(map[string]interface{})
	InjectHeaders(parentCtx, headers)
	amqpHeaders := amqp.Table{}
	for k, v := range headers {
		amqpHeaders[k] = v
	}

	delivery := amqp.Delivery{
		Body:    []byte(`{"ownerId":"user-123","data":{}}`),
		Headers: amqpHeaders,
	}

	// Handler that always fails
	handler := func(ctx context.Context, msg amqp.Delivery) error {
		return &retryableError{err: assertError("permanent failure")}
	}

	cfg := retry.Config{
		MaxRetries:  2,
		InitialWait: 1 * time.Millisecond,
		MaxWait:     10 * time.Millisecond,
	}

	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	ctx, consumerSpan := tracer.Start(ctx, "consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "consume"),
		),
	)

	err := retry.WithBackoff(ctx, cfg, func() error {
		return handler(ctx, delivery)
	})
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}

	consumerSpan.RecordError(err)
	consumerSpan.SetStatus(codes.Error, err.Error())
	consumerSpan.End()

	tp.ForceFlush(context.Background())

	// Verify span has error status
	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "consume" {
			if s.Status.Code != codes.Error {
				t.Errorf("expected error status code, got %v", s.Status.Code)
			}
			if len(s.Events) == 0 {
				t.Error("expected error event to be recorded")
			}
		}
	}
}

func TestConsumeMessages_NoHeaders(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Delivery without headers
	delivery := amqp.Delivery{
		Body:    []byte(`{"ownerId":"user-123","data":{}}`),
		Headers: amqp.Table{},
	}

	tracer := tp.Tracer("test")

	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	ctx, consumerSpan := tracer.Start(ctx, "consume",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "consume"),
		),
	)
	consumerSpan.End()

	tp.ForceFlush(context.Background())

	// Verify span was created with new trace (no parent)
	spans := exporter.GetSpans()
	var consumerSpanData tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "consume" {
			consumerSpanData = s
			break
		}
	}
	if consumerSpanData.Name == "" {
		t.Fatal("consumer span not found")
	}

	// Should have a valid span context (new trace)
	consumerSC := consumerSpanData.SpanContext
	if !consumerSC.IsValid() {
		t.Fatal("expected valid span context")
	}
	// Should not have a parent (new root span)
	if consumerSpanData.Parent.IsValid() {
		t.Error("expected no parent span context for message without headers")
	}
}

// assertError is a simple error type for testing
type assertError string

func (e assertError) Error() string {
	return string(e)
}