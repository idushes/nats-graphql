package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
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

	// GraphQL server
	srv := handler.NewDefaultServer(graph.NewExecutableSchema(graph.Config{
		Resolvers: &graph.Resolver{JS: js},
	}))

	http.Handle("/", playground.Handler("NATS GraphQL", "/query"))
	http.Handle("/query", middleware.Auth(srv))
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
