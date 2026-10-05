# Tasks

## 1. Message context and publication

- [x] 1.1 Add an AMQP header carrier for the configured OpenTelemetry propagator, including string and byte-string extraction; verify focused Go tests round-trip context and tolerate missing or invalid headers.
- [x] 1.2 Instrument `shared/messaging/RabbitMQ.PublishMessage` with a producer span, injected headers, safe routing attributes, and recorded/logged failures; verify focused tests preserve the JSON body, routing key, exchange, and persistent delivery mode.

## 2. Message consumption

- [x] 2.1 Instrument `RabbitMQ.ConsumeMessages` per delivery, passing the extracted consumer context through retries and follow-up publishes while retaining Ack/Nack behavior; verify focused tests cover trace parentage, successful retry, and final handler failure.
- [x] 2.2 Instrument the API Gateway's `QueueConsumer.Start` for per-delivery forwarding spans and recorded/logged decode or send errors without changing auto-ack behavior; verify focused tests cover a traced delivery and a delivery without headers.

## 3. Payment tracing and cross-service verification

- [x] 3.1 Initialize and shut down the existing tracing provider in the Payment Service with the `payment-service` identity and current Jaeger configuration; verify the Payment Service builds and a focused startup/configuration check confirms tracing is active before consumption.
- [x] 3.2 Run `go test ./shared/messaging/... ./services/payment-service/...` and build all four Go services; verify the commands pass and no `any` type or unlogged new error path was introduced.
- [x] 3.3 Run the project with `tilt up` and trace a trip through driver assignment and payment in Jaeger; verify producer and consumer spans remain connected across services, or document the specific unavailable external dependency if the smoke check cannot run.
