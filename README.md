# nats-graphql

GraphQL server for NATS JetStream administration. Provides an API to inspect and manage Key-Value stores and streams.

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

| Path       | Description                   | Auth Required                |
| ---------- | ----------------------------- | ---------------------------- |
| `/`        | GraphiQL playground           | No                           |
| `/query`   | GraphQL endpoint              | Yes (if `AUTH_TOKEN` is set) |
| `/healthz` | Health check (for K8s probes) | No                           |

### Example Queries

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
