package messaging

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// amqpHeaderCarrier adapts amqp091-go headers (amqp.Table) to the TextMapCarrier interface.
// RabbitMQ clients may store header values as string or []byte.
type amqpHeaderCarrier map[string]interface{}

func (c amqpHeaderCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	v := c[key]
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return ""
	}
}

func (c amqpHeaderCarrier) Set(key string, value string) {
	if c == nil {
		return
	}
	c[key] = value
}

func (c amqpHeaderCarrier) Keys() []string {
	if c == nil {
		return nil
	}
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectHeaders injects the current trace context from ctx into the AMQP headers.
func InjectHeaders(ctx context.Context, headers map[string]interface{}) {
	if headers == nil {
		return
	}
	carrier := amqpHeaderCarrier(headers)
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractHeaders extracts trace context from AMQP headers and returns a new context with the extracted span context.
func ExtractHeaders(ctx context.Context, headers map[string]interface{}) context.Context {
	if headers == nil {
		return ctx
	}
	carrier := amqpHeaderCarrier(headers)
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// Propagator returns the globally configured TextMapPropagator.
func Propagator() propagation.TextMapPropagator {
	return otel.GetTextMapPropagator()
}