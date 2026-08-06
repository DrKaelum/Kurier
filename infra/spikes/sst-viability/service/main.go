package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/sst/sst/v3/sdk/golang/resource"
)

var errQueueUnavailable = errors.New("queue configuration is unavailable")

type viabilityEvent struct {
	CorrelationID string    `json:"correlationId"`
	EventType     string    `json:"eventType"`
	Stage         string    `json:"stage"`
	Timestamp     time.Time `json:"timestamp"`
}

type queueSender interface {
	Send(context.Context, viabilityEvent) (string, error)
}

type sqsAPI interface {
	SendMessage(context.Context, *sqs.SendMessageInput, ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

type linkedQueueSender struct {
	client   sqsAPI
	queueURL string
}

type unavailableQueueSender struct {
	reason error
}

func main() {
	stage := envOrDefault("KURIER_STAGE", "unknown")
	sender := loadQueueSender(context.Background())
	server := &http.Server{
		Addr:              ":" + envOrDefault("PORT", "8080"),
		Handler:           routes(stage, sender),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("kurier SST viability service listening on %s for stage %s", server.Addr, stage)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("viability service failed: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("viability service shutdown failed: %v", err)
	}
}

func loadQueueSender(ctx context.Context) queueSender {
	queueURL, err := resource.Get("QueueAccess", "url")
	if err != nil {
		log.Printf("queue resource link unavailable: %v", err)
		return unavailableQueueSender{reason: err}
	}

	url, ok := queueURL.(string)
	if !ok || url == "" {
		err := errors.New("QueueAccess.url is missing or invalid")
		log.Printf("queue resource link unavailable: %v", err)
		return unavailableQueueSender{reason: err}
	}

	awsConfig, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Printf("AWS SDK configuration unavailable: %v", err)
		return unavailableQueueSender{reason: err}
	}

	return linkedQueueSender{
		client:   sqs.NewFromConfig(awsConfig),
		queueURL: url,
	}
}

func (sender linkedQueueSender) Send(ctx context.Context, event viabilityEvent) (string, error) {
	body, err := json.Marshal(event)
	if err != nil {
		return "", err
	}

	output, err := sender.client.SendMessage(ctx, &sqs.SendMessageInput{
		MessageBody: aws.String(string(body)),
		QueueUrl:    aws.String(sender.queueURL),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(output.MessageId), nil
}

func (sender unavailableQueueSender) Send(context.Context, viabilityEvent) (string, error) {
	return "", errors.Join(errQueueUnavailable, sender.reason)
}

func routes(stage string, sender queueSender) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": "sst-viability",
			"stage":   stage,
			"status":  "ok",
		})
	})
	mux.HandleFunc("POST /queue-test", func(w http.ResponseWriter, request *http.Request) {
		event, err := newViabilityEvent(stage)
		if err != nil {
			log.Printf("could not generate queue correlation ID: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "failed",
				"error":  "could not generate test metadata",
			})
			return
		}

		messageID, err := sender.Send(request.Context(), event)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, errQueueUnavailable) {
				status = http.StatusServiceUnavailable
			}
			log.Printf("queue test failed for correlation %s: %v", event.CorrelationID, err)
			writeJSON(w, status, map[string]string{
				"status":        "failed",
				"correlationId": event.CorrelationID,
				"error":         "queue send failed",
			})
			return
		}

		log.Printf("queue test sent for correlation %s", event.CorrelationID)
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":        "sent",
			"correlationId": event.CorrelationID,
			"messageId":     messageID,
		})
	})
	return mux
}

func newViabilityEvent(stage string) (viabilityEvent, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return viabilityEvent{}, err
	}
	return viabilityEvent{
		CorrelationID: hex.EncodeToString(random),
		EventType:     "kurier.sst.viability",
		Stage:         stage,
		Timestamp:     time.Now().UTC(),
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("JSON response failed: %v", err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
