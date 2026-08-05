package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

func main() {
	address := ":" + envOrDefault("PORT", "8080")
	server := &http.Server{
		Addr:    address,
		Handler: routes(),
	}

	log.Printf("kurier api listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "api",
			"status":  "ok",
		})
	})
	return mux
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
