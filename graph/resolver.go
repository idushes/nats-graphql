package graph

import "github.com/nats-io/nats.go/jetstream"

// Resolver holds application-wide dependencies.
type Resolver struct {
	JS jetstream.JetStream
}
