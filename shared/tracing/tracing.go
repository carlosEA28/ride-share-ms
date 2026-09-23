package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName string
	Environment string
	JaegerURL   string
}

func InitTracer(cfg Config) (func(context.Context) error, error) {
	// Exporter
	traceExporter, err := NewExporter(cfg.JaegerURL)
	if err != nil {
		return nil, err
	}

	//Trace Provider
	traceProvider, err := NewTracerProvider(cfg, traceExporter)
	if err != nil {
		return nil, fmt.Errorf("failed to create tracer provider: %w", err)
	}
	otel.SetTracerProvider(traceProvider)

	//propagator
	prop := NewPropagator()
	otel.SetTextMapPropagator(prop)

	return traceProvider.Shutdown, nil
}
func NewExporter(endpoint string) (sdktrace.SpanExporter, error) {
	return jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)))
}

func NewPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func NewTracerProvider(cfg Config, exporter sdktrace.SpanExporter) (*sdktrace.TracerProvider, error) {

	resource, err := resource.New(context.Background(), resource.WithAttributes(
		semconv.ServiceNameKey.String(cfg.ServiceName),
		semconv.DeploymentEnvironmentKey.String(cfg.Environment),
	))

	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter), sdktrace.WithResource(resource))

	return traceProvider, err
}

func GetTracer(serviceName string) trace.Tracer {
	return otel.GetTracerProvider().Tracer(serviceName)
}
