package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sst/sst/v3/sdk/golang/resource"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var errDatabaseUnavailable = errors.New("database configuration is unavailable")

type databaseConfig struct {
	Database  string
	Host      string
	Password  string
	Port      uint16
	SecretARN string
	Username  string
}

type databaseSecret struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

type databaseStore interface {
	Close()
	Health(context.Context) error
	MigrationStatus(context.Context) (int, error)
	RunMigrations(context.Context) error
	TestRecord(context.Context, string, time.Time) error
}

type postgresStore struct {
	pool *pgxpool.Pool
}

type unavailableStore struct{}

type migration struct {
	Version int
	Path    string
}

var migrations = []migration{
	{Version: 1, Path: "migrations/0001_create_spike_records.sql"},
}

func main() {
	ctx := context.Background()
	store := loadStore(ctx)
	if err := store.RunMigrations(ctx); err != nil {
		log.Print("database migration unavailable; service will report degraded health")
	} else {
		log.Print("database migrations are current")
	}

	server := &http.Server{
		Addr:              ":" + envOrDefault("PORT", "8080"),
		Handler:           routes(envOrDefault("KURIER_STAGE", "unknown"), store),
		ReadHeaderTimeout: 5 * time.Second,
	}

	signalContext, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		log.Printf("Kurier database viability service listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Print("database viability HTTP server failed")
			stop()
		}
	}()

	<-signalContext.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		log.Print("database viability HTTP shutdown failed")
	}
	store.Close()
}

func loadStore(ctx context.Context) databaseStore {
	databaseConfiguration, err := loadDatabaseConfig(ctx)
	if err != nil {
		return unavailableStore{}
	}

	poolConfig, err := createPoolConfig(databaseConfiguration)
	if err != nil {
		log.Print("secure database connection configuration unavailable")
		return unavailableStore{}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Print("database connection pool unavailable")
		return unavailableStore{}
	}
	return &postgresStore{pool: pool}
}

func loadDatabaseConfig(ctx context.Context) (databaseConfig, error) {
	host, err := linkedString("host")
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("link")
	}
	portValue, err := resource.Get("DatabaseAccess", "port")
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("link")
	}
	port, err := linkedPort(portValue)
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("link")
	}
	database, err := linkedString("database")
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("link")
	}
	username, err := linkedString("username")
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("link")
	}
	secretARN, err := linkedString("secretArn")
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("link")
	}

	awsConfiguration, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure("aws-config")
	}
	secretOutput, err := secretsmanager.NewFromConfig(awsConfiguration).GetSecretValue(
		ctx,
		&secretsmanager.GetSecretValueInput{SecretId: &secretARN},
	)
	if err != nil {
		return databaseConfig{}, databaseConfigurationFailure(
			secretFetchFailureCategory(err),
		)
	}
	if secretOutput.SecretString == nil {
		return databaseConfig{}, databaseConfigurationFailure("secret-fetch-empty")
	}

	var secret databaseSecret
	if err := json.Unmarshal([]byte(*secretOutput.SecretString), &secret); err != nil {
		return databaseConfig{}, databaseConfigurationFailure("secret-decode")
	}
	if secret.Password == "" || secret.Username == "" || secret.Username != username {
		return databaseConfig{}, databaseConfigurationFailure("secret-validation")
	}

	return databaseConfig{
		Database:  database,
		Host:      host,
		Password:  secret.Password,
		Port:      port,
		SecretARN: secretARN,
		Username:  username,
	}, nil
}

func createPoolConfig(databaseConfiguration databaseConfig) (*pgxpool.Config, error) {
	certificatePath := envOrDefault(
		"RDS_CA_BUNDLE",
		"/etc/ssl/certs/rds-global-bundle.pem",
	)
	certificateBundle, err := os.ReadFile(certificatePath)
	if err != nil {
		return nil, errors.New("RDS certificate bundle unavailable")
	}
	rootCertificates := x509.NewCertPool()
	if !rootCertificates.AppendCertsFromPEM(certificateBundle) {
		return nil, errors.New("RDS certificate bundle is invalid")
	}

	connectionURL := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			databaseConfiguration.Username,
			databaseConfiguration.Password,
		),
		Host: net.JoinHostPort(
			databaseConfiguration.Host,
			strconv.Itoa(int(databaseConfiguration.Port)),
		),
		Path: "/" + databaseConfiguration.Database,
	}
	query := connectionURL.Query()
	query.Set("sslmode", "verify-full")
	connectionURL.RawQuery = query.Encode()

	poolConfig, err := pgxpool.ParseConfig(connectionURL.String())
	if err != nil {
		return nil, errors.New("database connection configuration is invalid")
	}
	poolConfig.ConnConfig.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    rootCertificates,
		ServerName: databaseConfiguration.Host,
	}
	poolConfig.MaxConns = 2
	poolConfig.MinConns = 0
	poolConfig.MaxConnIdleTime = time.Minute
	return poolConfig, nil
}

func (store *postgresStore) Close() {
	store.pool.Close()
}

func (store *postgresStore) Health(ctx context.Context) error {
	healthContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return store.pool.Ping(healthContext)
}

func (store *postgresStore) RunMigrations(ctx context.Context) error {
	transaction, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	if _, err := transaction.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sst_db_viability_migrations (
			version integer PRIMARY KEY,
			applied_at timestamptz NOT NULL
		)
	`); err != nil {
		return err
	}

	for _, candidate := range migrations {
		var applied bool
		if err := transaction.QueryRow(
			ctx,
			"SELECT EXISTS (SELECT 1 FROM sst_db_viability_migrations WHERE version = $1)",
			candidate.Version,
		).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}

		statement, err := migrationFiles.ReadFile(candidate.Path)
		if err != nil {
			return err
		}
		if _, err := transaction.Exec(ctx, string(statement)); err != nil {
			return err
		}
		if _, err := transaction.Exec(
			ctx,
			"INSERT INTO sst_db_viability_migrations (version, applied_at) VALUES ($1, $2)",
			candidate.Version,
			time.Now().UTC(),
		); err != nil {
			return err
		}
	}
	return transaction.Commit(ctx)
}

func (store *postgresStore) MigrationStatus(ctx context.Context) (int, error) {
	var version int
	err := store.pool.QueryRow(
		ctx,
		"SELECT COALESCE(MAX(version), 0) FROM sst_db_viability_migrations",
	).Scan(&version)
	return version, err
}

func (store *postgresStore) TestRecord(
	ctx context.Context,
	correlationID string,
	createdAt time.Time,
) error {
	transaction, err := store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = transaction.Rollback(ctx)
	}()

	const eventType = "kurier.sst.database-viability"
	if _, err := transaction.Exec(
		ctx,
		`INSERT INTO sst_db_viability_records
			(correlation_id, event_type, created_at) VALUES ($1, $2, $3)`,
		correlationID,
		eventType,
		createdAt,
	); err != nil {
		return err
	}

	var retrievedEventType string
	var retrievedTime time.Time
	if err := transaction.QueryRow(
		ctx,
		`SELECT event_type, created_at FROM sst_db_viability_records
			WHERE correlation_id = $1`,
		correlationID,
	).Scan(&retrievedEventType, &retrievedTime); err != nil {
		return err
	}
	if retrievedEventType != eventType || !retrievedTime.Equal(createdAt) {
		return errors.New("retrieved spike record did not match")
	}

	result, err := transaction.Exec(
		ctx,
		"DELETE FROM sst_db_viability_records WHERE correlation_id = $1",
		correlationID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("spike record was not deleted")
	}
	return transaction.Commit(ctx)
}

func (unavailableStore) Close() {}

func (unavailableStore) Health(context.Context) error {
	return errDatabaseUnavailable
}

func (unavailableStore) MigrationStatus(context.Context) (int, error) {
	return 0, errDatabaseUnavailable
}

func (unavailableStore) RunMigrations(context.Context) error {
	return errDatabaseUnavailable
}

func (unavailableStore) TestRecord(context.Context, string, time.Time) error {
	return errDatabaseUnavailable
}

func routes(stage string, store databaseStore) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /service-health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": "ok",
			"stage":   stage,
		})
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, request *http.Request) {
		if err := store.Health(request.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"database": "unavailable",
				"service":  "ok",
				"stage":    stage,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"database": "ok",
			"service":  "ok",
			"stage":    stage,
		})
	})
	mux.HandleFunc("GET /migration-status", func(w http.ResponseWriter, request *http.Request) {
		version, err := store.MigrationStatus(request.Context())
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"database": "unavailable",
				"service":  "ok",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"currentVersion": version,
			"status":         "current",
		})
	})
	mux.HandleFunc("POST /migrate", func(w http.ResponseWriter, request *http.Request) {
		if err := store.RunMigrations(request.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "current"})
	})
	mux.HandleFunc("POST /database-test", func(w http.ResponseWriter, request *http.Request) {
		correlationID, err := newCorrelationID()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "failed",
			})
			return
		}
		if err := store.TestRecord(
			request.Context(),
			correlationID,
			time.Now().UTC().Truncate(time.Microsecond),
		); err != nil {
			log.Printf("database record test failed for correlation %s", correlationID)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"correlationId": correlationID,
				"status":        "failed",
			})
			return
		}
		log.Printf("database record test passed for correlation %s", correlationID)
		writeJSON(w, http.StatusOK, map[string]string{
			"correlationId": correlationID,
			"status":        "inserted-retrieved-deleted",
		})
	})
	return mux
}

func databaseConfigurationFailure(category string) error {
	log.Printf("database configuration unavailable category=%s", category)
	return errDatabaseUnavailable
}

func secretFetchFailureCategory(err error) string {
	type codedAPIError interface {
		ErrorCode() string
	}
	var apiError codedAPIError
	if !errors.As(err, &apiError) {
		return "secret-fetch-transport"
	}
	if apiError.ErrorCode() == "AccessDeniedException" {
		return "secret-fetch-access-denied"
	}
	return "secret-fetch-api"
}

func linkedString(property string) (string, error) {
	value, err := resource.Get("DatabaseAccess", property)
	if err != nil {
		return "", errDatabaseUnavailable
	}
	text, ok := value.(string)
	if !ok || text == "" {
		return "", errDatabaseUnavailable
	}
	return text, nil
}

func linkedPort(value any) (uint16, error) {
	var port int
	switch typed := value.(type) {
	case float64:
		port = int(typed)
	case int:
		port = typed
	case string:
		parsed, err := strconv.Atoi(typed)
		if err != nil {
			return 0, errDatabaseUnavailable
		}
		port = parsed
	default:
		return 0, errDatabaseUnavailable
	}
	if port < 1 || port > 65535 {
		return 0, errDatabaseUnavailable
	}
	return uint16(port), nil
}

func newCorrelationID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate correlation ID: %w", err)
	}
	return hex.EncodeToString(random), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Print("JSON response failed")
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
