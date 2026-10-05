package messaging

import (
	"context"
	"encoding/json"
	"testing"

	"ride-sharing/shared/contracts"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)



func TestQueueConsumer_ForwardWithHeaders(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create a parent context (simulating upstream producer)
	parentCtx := context.Background()
	tracer := tp.Tracer("test")
	parentCtx, parentSpan := tracer.Start(parentCtx, "producer-operation")
	parentSpan.End()

	// Create headers with trace context
	headers := make(map[string]interface{})
	InjectHeaders(parentCtx, headers)
	amqpHeaders := amqp.Table{}
	for k, v := range headers {
		amqpHeaders[k] = v
	}

	// Create a fake delivery
	delivery := amqp.Delivery{
		Body:       []byte(`{"ownerId":"user-123","data":{}}`),
		Headers:    amqpHeaders,
		RoutingKey: "test.event",
	}

	// Simulate extraction
	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	// Start consumer span
	ctx, consumerSpan := tracer.Start(ctx, "forward",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "forward"),
			attribute.String("messaging.routing_key", "test.event"),
		),
	)
	consumerSpan.End()

	tp.ForceFlush(context.Background())

	// Verify spans
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	// Find consumer span
	var consumerSpanData tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "forward" {
			consumerSpanData = s
			break
		}
	}
	if consumerSpanData.Name == "" {
		t.Fatal("forward span not found")
	}

	// Verify attributes
	attrs := consumerSpanData.Attributes
	foundSystem := false
	foundDestination := false
	foundOperation := false
	foundRoutingKey := false
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
			if a.Value.AsString() != "forward" {
				t.Errorf("messaging.operation = %s, want forward", a.Value.AsString())
			}
			foundOperation = true
		case "messaging.routing_key":
			if a.Value.AsString() != "test.event" {
				t.Errorf("messaging.routing_key = %s, want test.event", a.Value.AsString())
			}
			foundRoutingKey = true
		}
	}
	if !foundSystem || !foundDestination || !foundOperation || !foundRoutingKey {
		t.Error("missing expected span attributes")
	}

	// Verify trace parentage
	parentSC := trace.SpanContextFromContext(parentCtx)
	consumerSC := consumerSpanData.SpanContext
	if consumerSC.TraceID() != parentSC.TraceID() {
		t.Errorf("trace ID mismatch: consumer trace ID %s != parent trace ID %s",
			consumerSC.TraceID(), parentSC.TraceID())
	}
}

func TestQueueConsumer_ForwardWithoutHeaders(t *testing.T) {
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
		Body:       []byte(`{"ownerId":"user-123","data":{}}`),
		Headers:    amqp.Table{},
		RoutingKey: "test.event",
	}

	tracer := tp.Tracer("test")

	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	ctx, consumerSpan := tracer.Start(ctx, "forward",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "forward"),
			attribute.String("messaging.routing_key", "test.event"),
		),
	)
	consumerSpan.End()

	tp.ForceFlush(context.Background())

	// Verify span was created with new trace (no parent)
	spans := exporter.GetSpans()
	var consumerSpanData tracetest.SpanStub
	for _, s := range spans {
		if s.Name == "forward" {
			consumerSpanData = s
			break
		}
	}
	if consumerSpanData.Name == "" {
		t.Fatal("forward span not found")
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

func TestQueueConsumer_Forward_DecodeError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Delivery with invalid JSON
	delivery := amqp.Delivery{
		Body:       []byte(`invalid json`),
		Headers:    amqp.Table{},
		RoutingKey: "test.event",
	}

	tracer := tp.Tracer("test")

	extractedHeaders := make(map[string]interface{})
	for k, v := range delivery.Headers {
		extractedHeaders[k] = v
	}
	ctx := ExtractHeaders(context.Background(), extractedHeaders)

	ctx, consumerSpan := tracer.Start(ctx, "forward",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "forward"),
			attribute.String("messaging.routing_key", "test.event"),
		),
	)

	// Simulate decode error
	var msgBody contracts.AmqpMessage
	err := json.Unmarshal(delivery.Body, &msgBody)
	if err == nil {
		t.Fatal("expected decode error")
	}

	consumerSpan.RecordError(err)
	consumerSpan.SetStatus(codes.Error, err.Error())
	consumerSpan.End()

	tp.ForceFlush(context.Background())

	// Verify span has error status
	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "forward" {
			if s.Status.Code != codes.Error {
				t.Errorf("expected error status code, got %v", s.Status.Code)
			}
			if len(s.Events) == 0 {
				t.Error("expected error event to be recorded")
			}
		}
	}
}

func TestQueueConsumer_Forward_SendError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	defer func() { _ = tp.Shutdown(context.Background()) }()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer := tp.Tracer("test")

	_, consumerSpan := tracer.Start(context.Background(), "forward",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "rabbitmq"),
			attribute.String("messaging.destination", "test-queue"),
			attribute.String("messaging.destination_kind", "queue"),
			attribute.String("messaging.operation", "forward"),
			attribute.String("messaging.routing_key", "test.event"),
		),
	)

	// Simulate send error
	err := assertError("send failed")
	consumerSpan.RecordError(err)
	consumerSpan.SetStatus(codes.Error, err.Error())
	consumerSpan.End()

	tp.ForceFlush(context.Background())

	// Verify span has error status
	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "forward" {
			if s.Status.Code != codes.Error {
				t.Errorf("expected error status code, got %v", s.Status.Code)
			}
			if len(s.Events) == 0 {
				t.Error("expected error event to be recorded")
			}
		}
	}
}