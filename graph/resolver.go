package graph

import (
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Resolver holds application-wide dependencies.
type Resolver struct {
	NC *nats.Conn
	JS jetstream.JetStream
}
