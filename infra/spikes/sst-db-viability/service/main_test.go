package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	healthErr     error
	migrationErr  error
	recordErr     error
	recordTested  bool
	statusVersion int
}

func (*fakeStore) Close() {}

func (store *fakeStore) Health(context.Context) error {
	return store.healthErr
}

func (store *fakeStore) MigrationStatus(context.Context) (int, error) {
	return store.statusVersion, store.migrationErr
}

func (store *fakeStore) RunMigrations(context.Context) error {
	return store.migrationErr
}

func (store *fakeStore) TestRecord(
	_ context.Context,
	correlationID string,
	createdAt time.Time,
) error {
	store.recordTested = correlationID != "" && !createdAt.IsZero()
	return store.recordErr
}

func TestHealthDistinguishesDatabaseConnectivity(t *testing.T) {
	store := &fakeStore{}
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	routes("db-viability", store).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["service"] != "ok" || body["database"] != "ok" {
		t.Fatalf("unexpected health response: %#v", body)
	}
}

func TestHealthFailsSafelyWithoutDatabase(t *testing.T) {
	store := &fakeStore{healthErr: errDatabaseUnavailable}
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	routes("db-viability", store).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusServiceUnavailable,
			response.Code,
		)
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["service"] != "ok" || body["database"] != "unavailable" {
		t.Fatalf("unexpected health response: %#v", body)
	}
}

func TestDatabaseRecordLifecycle(t *testing.T) {
	store := &fakeStore{}
	request := httptest.NewRequest(http.MethodPost, "/database-test", nil)
	response := httptest.NewRecorder()

	routes("db-viability", store).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if !store.recordTested {
		t.Fatal("expected a generated correlation ID and timestamp")
	}
}

func TestMigrationStatus(t *testing.T) {
	store := &fakeStore{statusVersion: 1}
	request := httptest.NewRequest(http.MethodGet, "/migration-status", nil)
	response := httptest.NewRecorder()

	routes("db-viability", store).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["currentVersion"] != float64(1) {
		t.Fatalf("unexpected migration status: %#v", body)
	}
}

func TestMigrationCatalog(t *testing.T) {
	if len(migrations) != 1 || migrations[0].Version != 1 {
		t.Fatalf("unexpected migration catalog: %#v", migrations)
	}
	statement, err := migrationFiles.ReadFile(migrations[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(statement) == 0 {
		t.Fatal("migration file is empty")
	}
}

func TestDatabaseFailureIsRedacted(t *testing.T) {
	store := &fakeStore{recordErr: errors.New("secret connection details")}
	request := httptest.NewRequest(http.MethodPost, "/database-test", nil)
	response := httptest.NewRecorder()

	routes("db-viability", store).ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusServiceUnavailable,
			response.Code,
		)
	}
	if response.Body.String() == "" {
		t.Fatal("expected a redacted failure response")
	}
	if strings.Contains(response.Body.String(), "secret connection details") {
		t.Fatal("response exposed an internal database error")
	}
}
