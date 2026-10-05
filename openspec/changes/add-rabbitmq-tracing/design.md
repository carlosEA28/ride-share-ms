# Design

## Context

See `proposal.md` for motivation and `specs/messaging-tracing/spec.md` for the behavior contract. `shared/tracing` configures an OpenTelemetry provider, W3C trace context plus baggage propagation, and Jaeger export for the API Gateway, Trip Service, and Driver Service. `shared/messaging/RabbitMQ.PublishMessage` serializes the existing JSON envelope and calls `PublishWithContext`, but sets no AMQP headers. `ConsumeMessages` supplies one `context.Background()` to all deliveries; the API Gateway's `QueueConsumer` consumes deliveries separately. The Payment Service does not initialize a tracer.

## Goals / Non-Goals

**Goals:**

- Use one tracing approach at both RabbitMQ consumer entry points and the shared publisher, preserving trace continuity across message chains.
- Keep message bodies, routing, acknowledgements, retries, and existing application handler signatures compatible.
- Keep tracing metadata bounded to standard propagation headers and low-cardinality operation attributes.

**Non-Goals:**

- Redesign the event schema, queue topology, retry policy, or WebSocket tracing beyond the RabbitMQ-to-WebSocket forwarding operation.
- Add a second tracing backend or replace the existing Jaeger exporter.

## Decisions

### Propagate through AMQP headers

Use `otel.GetTextMapPropagator()` to inject into `amqp.Publishing.Headers` and extract from `amqp.Delivery.Headers`. Add a small carrier adapter for AMQP table values, reading the string and byte-string forms used by RabbitMQ clients. Create a fresh headers table for each publish. Inject the producer span context, not only the incoming caller context, so consumer spans connect to that publish operation. Keep the JSON envelope untouched. An alternative is to add trace fields to `contracts.AmqpMessage`; that would alter the application payload and require changes to every handler.

### Instrument at the shared transport boundary

Start a producer span in `PublishMessage` before serialization and publishing, pass its derived context to `PublishWithContext`, and end it after the publish call. In `ConsumeMessages`, extract separately for each delivery and start a consumer span before the retry loop. Pass the derived context into each handler attempt and end the span after the existing Ack/Nack path. In `QueueConsumer.Start`, do the same around decoding and WebSocket forwarding; retain its current auto-ack behavior. An alternative is to instrument every service event handler, which risks inconsistent coverage and misses the Gateway's separate consumer.

Use span kind producer/consumer, stable exchange, queue, and routing-key attributes, and record operation errors with an error status. Keep existing logs for errors, including failures encountered in new tracing code. Do not attach message bodies, user IDs, driver IDs, Stripe metadata, or raw headers to spans. Missing or malformed trace headers yield a new root span; message handling remains unaffected. Do not report a transient retry attempt as a final failure when a later attempt succeeds; the existing retry logger still records those attempts.

### Initialize tracing in the Payment Service

Call the existing `tracing.InitTracer` during Payment Service startup with `payment-service`, the current environment variable conventions, and Jaeger endpoint default used by the other services. Shut down the provider on service exit and log initialization and shutdown errors. The existing default Jaeger endpoint means development manifests need no new environment variable to enable export. An alternative is to rely on the process-wide no-op provider, which would propagate incoming context but produce no Payment Service spans.

### Verify context continuity without a broker dependency

Use focused Go tests with an in-memory span exporter and OpenTelemetry propagator to verify header round trips, span parentage across publish and consume operations, absent or malformed headers, error status, and unchanged publish metadata. Because `RabbitMQ` holds a concrete channel, isolate the carrier and span lifecycle logic enough to test it without a live RabbitMQ instance; add a development smoke check in Jaeger for the full trip-to-payment flow. Do not make normal unit tests depend on Kubernetes or Stripe.

## Risks / Trade-offs

- [Older queued messages have no trace headers] -> Extraction starts a new trace, and payload processing remains compatible.
- [A message is retried or delivered again] -> Keep one consumer span per delivery; record final failure and preserve current retry and Ack/Nack behavior.
- [Instrumentation adds attributes or headers that expose data] -> Use only routing metadata and standard propagation fields; exclude payload and identity fields.
- [The Payment Service cannot export to Jaeger] -> Keep its existing startup error handling pattern and log tracing initialization or shutdown failures; deployment can be rolled back with the previous image.

## Migration Plan

1. Deploy the shared messaging instrumentation to publishers and consumers, including the Gateway's consumer path, then deploy the Payment Service tracer initialization.
2. Verify a new trip trace in Jaeger across Gateway, Trip, Driver, and Payment services. Older messages continue to process as independent traces.
3. Roll back service images if required. Added AMQP headers are ignored by older consumers, so no queue or payload migration is needed.
