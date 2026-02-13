package middleware

import (
	"net/http"
	"os"
	"strings"
)

// Auth returns middleware that checks the Authorization token.
// If AUTH_TOKEN env is not set, all requests are allowed.
func Auth(next http.Handler) http.Handler {
	token := os.Getenv("AUTH_TOKEN")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("Authorization")
		value := strings.TrimPrefix(header, "Bearer ")

		if value != token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errors":[{"message":"Unauthorized: invalid or missing Bearer token"}]}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
