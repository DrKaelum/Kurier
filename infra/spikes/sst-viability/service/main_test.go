package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeQueueSender struct {
	event     viabilityEvent
	messageID string
	err       error
}

func (sender *fakeQueueSender) Send(_ context.Context, event viabilityEvent) (string, error) {
	sender.event = event
	return sender.messageID, sender.err
}

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	routes("viability", &fakeQueueSender{}).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["stage"] != "viability" || body["status"] != "ok" {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestQueueSend(t *testing.T) {
	sender := &fakeQueueSender{messageID: "message-id"}
	request := httptest.NewRequest(http.MethodPost, "/queue-test", nil)
	response := httptest.NewRecorder()

	routes("viability", sender).ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.Code)
	}
	if sender.event.EventType != "kurier.sst.viability" || sender.event.Stage != "viability" {
		t.Fatalf("unexpected test event: %#v", sender.event)
	}
	if sender.event.CorrelationID == "" || sender.event.Timestamp.IsZero() {
		t.Fatalf("test event is missing generated metadata: %#v", sender.event)
	}
}

func TestQueueConfigurationUnavailable(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/queue-test", nil)
	response := httptest.NewRecorder()
	sender := unavailableQueueSender{reason: errors.New("not linked")}

	routes("viability", sender).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.Code)
	}
}
