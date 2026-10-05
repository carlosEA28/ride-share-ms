# messaging-tracing Specification

## Purpose
Preserve distributed trace continuity and expose RabbitMQ message handling across the ride-sharing services, from publication through downstream processing.

## Requirements

### Requirement: Trace context accompanies published messages
The system SHALL attach the active distributed trace context to each RabbitMQ message it publishes without changing the message body, routing key, or delivery behavior.

#### Scenario: Publication within an active trace
- **WHEN** a service publishes a RabbitMQ message while handling a traced request or message
- **THEN** the message carries context that allows its consumers to continue that trace

#### Scenario: Publication without an active trace
- **WHEN** a service publishes a RabbitMQ message without an upstream trace
- **THEN** publication succeeds and the message can start a new trace

### Requirement: RabbitMQ operations appear in traces
The system SHALL record producer and consumer operations for RabbitMQ messages, including failures, with enough routing information to identify the operation without recording message payloads or sensitive identifiers as span attributes.

#### Scenario: Successful publish and consume
- **WHEN** a message is published and handled successfully
- **THEN** its producer and consumer operations are visible in the tracing backend with their respective service identities and routing metadata

#### Scenario: Failed publication or handling
- **WHEN** a publish or message handling operation fails
- **THEN** its operation records the failure and existing error logging and message acknowledgement behavior are preserved

### Requirement: Consumers continue incoming traces
The system SHALL use valid trace context from each incoming RabbitMQ message for its handling operation and any follow-up message published by that handler.

#### Scenario: Chained trip events
- **WHEN** a traced trip event causes driver or payment work that publishes another message
- **THEN** the downstream work remains connected to the originating trace across services

#### Scenario: Message without usable trace context
- **WHEN** a consumer receives a message with absent or invalid trace context
- **THEN** it processes the message normally under a new trace

#### Scenario: Gateway forwards an event to WebSocket
- **WHEN** the API Gateway consumes a RabbitMQ event for a connected user
- **THEN** its forwarding operation is visible as part of the message's trace

### Requirement: Payment processing exports message traces
The Payment Service SHALL initialize the project's tracing provider before processing RabbitMQ messages so its operations can be exported with the payment service identity.

#### Scenario: Create payment session
- **WHEN** a traced payment command reaches the Payment Service
- **THEN** its processing and subsequent payment event are visible in the same distributed trace
