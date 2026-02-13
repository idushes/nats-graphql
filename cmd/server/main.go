package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/joho/godotenv"

	"nats-graphql/graph"
	"nats-graphql/middleware"
	natsclient "nats-graphql/nats"
	"nats-graphql/playground"
)

func main() {
	// Load .env file if present (ignored in production/k8s)
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Connect to NATS
	nc, js, err := natsclient.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	log.Printf("Connected to NATS at %s", nc.ConnectedUrl())

	// GraphQL server with WebSocket support for subscriptions
	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{NC: nc, JS: js},
	}))
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
	})
	srv.Use(extension.Introspection{})

	http.Handle("/", playground.Handler("NATS GraphQL", "/query"))
	http.Handle("/query", middleware.CORS(middleware.Auth(srv)))
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !nc.IsConnected() {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("NATS disconnected"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("GraphQL playground: http://localhost:%s/", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
