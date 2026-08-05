package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	address := ":" + envOrDefault("HEALTH_PORT", "8081")
	server := &http.Server{Addr: address, Handler: healthRoutes()}

	go func() {
		log.Printf("kurier worker ready; health endpoint listening on %s", address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("worker health server failed: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Print("kurier worker stopping")
	_ = server.Shutdown(context.Background())
}

func healthRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service": "worker",
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
