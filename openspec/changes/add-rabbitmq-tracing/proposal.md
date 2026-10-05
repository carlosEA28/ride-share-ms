# Proposal

## Why

HTTP and gRPC requests already produce OpenTelemetry traces, but trace context is lost when work crosses RabbitMQ. This makes a trip, driver assignment, and payment flow appear as disconnected operations in Jaeger.

## What Changes

- Propagate trace context through RabbitMQ message headers and create producer and consumer spans for the existing publish and consume paths.
- Continue the extracted trace while handlers process messages, retry work, and publish follow-up events.
- Initialize the existing tracer in the Payment Service so its RabbitMQ work is visible in Jaeger.
- Cover both service consumers and the API Gateway's RabbitMQ-to-WebSocket consumer, including messages without tracing headers.
- Add focused automated checks for propagation, span relationships, error reporting, and unchanged message payloads and routing.

## Capabilities

### New Capabilities

- `messaging-tracing`: Distributed trace continuity and operation visibility across RabbitMQ publishers and consumers.

### Modified Capabilities

None.

## Impact

- Affects `shared/messaging`, the Payment Service startup, and relevant tracing tests; existing HTTP and gRPC instrumentation remains the upstream trace source.
- Adds AMQP message headers for trace propagation without changing routing keys, exchanges, queue bindings, or JSON message bodies.
- Uses the existing OpenTelemetry and Jaeger setup; no new external service is required.
