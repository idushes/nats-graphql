# nats-graphql

GraphQL server for NATS JetStream administration. Provides an API to inspect and manage Key-Value stores and streams.

## Features

**Key-Value Stores**

- `keyValues` — list all KV buckets with config and stats
- `kvKeys` — list keys in a bucket
- `kvGet` — read a key (returns null if missing)
- `kvCreate` — create a new bucket (optional: history, ttl, storage)
- `kvPut` — create or update a key
- `kvDelete` — soft-delete a key (leaves tombstone marker)
- `kvPurge` — hard-delete a key (removes key + all history)
- `kvDeleteBucket` — delete an entire bucket

**Streams**

- `streams` — list all streams with config and runtime state
- `streamCreate` — create a new stream (subjects, retention, storage, maxMsgs, maxBytes, replicas)
- `streamCopy` — create a stream that aggregates messages from multiple source streams
- `streamDelete` — delete a stream
- `streamMessages` — read messages with flexible filtering:
  - `startSeq` — start from sequence number
  - `startTime` / `endTime` — time range (RFC3339)
  - `subject` — filter by subject pattern
  - `last` — limit results (max 100)
- `publish` — publish a message to any subject (max 1MB)
- `publishScheduled` — delayed publish after N seconds (fire-and-forget)

**Consumers**

- `consumers` — list all consumers on a stream
- `consumerInfo` — get detailed info about a specific consumer
- `consumerCreate` — create or update a durable pull consumer (filterSubject, deliverPolicy, ackPolicy, etc.)
- `consumerDelete` — delete a consumer
- `consumerPause` — pause a consumer until a specified time
- `consumerResume` — resume a paused consumer

**Subscriptions (WebSocket)**

- `streamSubscribe` — real-time message streaming via `graphql-transport-ws`
  - Optional `subject` filter

**Infrastructure**

- GraphiQL playground with example queries and header editor
- `/healthz` — liveness probe (always 200)
- `/readyz` — readiness probe (checks NATS connection)
- Request logging (method, path, status, duration)
- Docker-ready (multi-stage Dockerfile)

**Security**

- Optional Bearer token auth (`AUTH_TOKEN`)
- CORS (all origins)

## Quick Start

```bash
cp .env.example .env
go run ./cmd/server/
```

Open `http://localhost:8080/` — GraphiQL playground with ready-to-use example queries.

## Configuration

| Variable     | Default                 | Description                                                                  |
| ------------ | ----------------------- | ---------------------------------------------------------------------------- |
| `NATS_URL`   | `nats://localhost:4222` | NATS server address                                                          |
| `PORT`       | `8080`                  | HTTP server port                                                             |
| `AUTH_TOKEN` | _(not set)_             | Auth token. If set, `/query` requires `Authorization: Bearer <token>` header |

Variables are read from `.env` file (convenient for local development) and from environment (for Kubernetes).

## API

### Endpoints

| Path       | Description                        | Auth Required                |
| ---------- | ---------------------------------- | ---------------------------- |
| `/`        | GraphiQL playground                | No                           |
| `/query`   | GraphQL endpoint                   | Yes (if `AUTH_TOKEN` is set) |
| `/healthz` | Liveness probe (K8s)               | No                           |
| `/readyz`  | Readiness probe (K8s, checks NATS) | No                           |

### Example Queries

> If `AUTH_TOKEN` is set, add `-H "Authorization: Bearer <token>"` to all requests:
>
> ```bash
> curl -X POST http://localhost:8080/query \
>   -H "Authorization: Bearer your-secret-token" \
>   -H "Content-Type: application/json" \
>   -d '{"query":"{ keyValues { bucket } }"}'
> ```

**List KV stores:**

```graphql
{
  keyValues {
    bucket
    history
    ttl
    storage
    bytes
    values
    isCompressed
  }
}
```

**List keys in a KV bucket:**

```graphql
{
  kvKeys(bucket: "my-bucket")
}
```

**Get value for a specific key:**

```graphql
{
  kvGet(bucket: "my-bucket", key: "my-key") {
    key
    value
    revision
    created
  }
}
```

**Put a value (mutation):**

```graphql
mutation {
  kvPut(bucket: "my-bucket", key: "my-key", value: "hello") {
    key
    value
    revision
    created
  }
}
```

**Delete a key (mutation):**

```graphql
mutation {
  kvDelete(bucket: "my-bucket", key: "my-key")
}
```

**List streams:**

```graphql
{
  streams {
    name
    subjects
    retention
    storage
    replicas
    maxConsumers
    maxMsgs
    maxBytes
    messages
    bytes
    consumers
    created
    sources {
      name
      lag
      active
      filterSubject
    }
  }
}
```

**Create a stream that aggregates from multiple sources (mutation):**

```graphql
mutation {
  streamCopy(
    name: "all-orders"
    sources: [
      { name: "orders-eu" }
      { name: "orders-us", filterSubject: "orders.paid" }
    ]
  ) {
    name
    sources {
      name
      lag
    }
  }
}
```

**Read last N messages from a stream:**

```graphql
{
  streamMessages(stream: "my-stream", last: 5) {
    sequence
    subject
    data
    published
  }
}
```

**Read messages with filters (all optional, can be combined):**

```graphql
{
  streamMessages(
    stream: "my-stream"
    last: 10
    startSeq: 100
    startTime: "2026-01-01T00:00:00Z"
    endTime: "2026-12-31T23:59:59Z"
    subject: "orders.new"
  ) {
    sequence
    subject
    data
    published
  }
}
```

| Filter      | Type     | Description                        |
| ----------- | -------- | ---------------------------------- |
| `last`      | `Int!`   | Max messages (default 10, cap 100) |
| `startSeq`  | `Int`    | Start from sequence number         |
| `startTime` | `String` | Start from timestamp (RFC3339)     |
| `endTime`   | `String` | Stop at timestamp (RFC3339)        |
| `subject`   | `String` | Filter by subject                  |

**Publish a message (mutation):**

```graphql
mutation {
  publish(subject: "orders.new", data: "{\"id\": 1}") {
    stream
    sequence
  }
}
```

**Subscribe to new messages in real-time (WebSocket):**

```graphql
subscription {
  streamSubscribe(stream: "my-stream", subject: "orders.>") {
    sequence
    subject
    data
    published
  }
}
```

Subscriptions use the `graphql-transport-ws` WebSocket protocol. The optional `subject` parameter filters messages by subject pattern.

**List consumers on a stream:**

```graphql
{
  consumers(stream: "my-stream") {
    name
    stream
    deliverPolicy
    ackPolicy
    numPending
    numAckPending
    paused
  }
}
```

**Create a consumer (mutation):**

```graphql
mutation {
  consumerCreate(
    stream: "my-stream"
    name: "my-consumer"
    filterSubject: "orders.>"
    deliverPolicy: "new"
    ackPolicy: "explicit"
    maxDeliver: 5
    description: "Process new orders"
  ) {
    name
    stream
    deliverPolicy
    ackPolicy
    maxDeliver
  }
}
```

**Pause a consumer (mutation):**

```graphql
mutation {
  consumerPause(
    stream: "my-stream"
    name: "my-consumer"
    pauseUntil: "2026-02-15T14:00:00Z"
  )
}
```

**Resume a consumer (mutation):**

```graphql
mutation {
  consumerResume(stream: "my-stream", name: "my-consumer")
}
```

**Delete a consumer (mutation):**

```graphql
mutation {
  consumerDelete(stream: "my-stream", name: "my-consumer")
}
```

### Safety Limits

| Limit                 | Value            | Description                                         |
| --------------------- | ---------------- | --------------------------------------------------- |
| `streamMessages` max  | **100 messages** | Hard cap per request, returns error if `last > 100` |
| `publish` max payload | **1 MB**         | Returns error if payload exceeds 1MB                |

**curl with token:**

```bash
curl http://localhost:8080/query \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer your-secret-token' \
  -d '{"query":"{ kvKeys(bucket: \"my-bucket\") }"}'
```

## Docker

```bash
docker build -t nats-graphql .
docker run -p 8080:8080 -e NATS_URL=nats://host.docker.internal:4222 nats-graphql
```

## Project Structure

```
├── cmd/server/main.go        # Entrypoint
├── graph/
│   ├── schema.graphqls       # GraphQL schema
│   ├── resolver.go           # Resolver with dependencies
│   ├── schema.resolvers.go   # Query implementations
│   ├── generated.go          # Generated runtime (gqlgen)
│   └── model/                # Generated models
├── nats/client.go            # NATS connection
├── middleware/auth.go        # Token auth middleware
├── playground/handler.go     # GraphiQL with examples
├── Dockerfile                # Multi-stage build
└── gqlgen.yml                # Code generation config
```

## Development

Regenerate code after schema changes:

```bash
go run github.com/99designs/gqlgen generate
```
