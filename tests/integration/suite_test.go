//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
)

// testDBPool is a shared connection pool for all integration tests.
var testDBPool *pgxpool.Pool

const (
	// apiEndpointURL is the base URL the suite exercises.
	apiEndpointURL = "http://localhost:8080/api/v1"

	// apiHealthURL is the readiness probe for the API server.
	apiHealthURL = apiEndpointURL + "/health/status"

	// mailServerEndpointURL is the endpoint for email testing.
	mailServerEndpointURL = "http://localhost:8025/api/v1"
	verifyEmailAddress    = config.DefaultMailSenderAddress

	// Database connection parameters.
	dbHost               = "localhost"
	dbPort               = 5432
	dbSSLMode            = "disable"
	dbName               = "go-rest-api-service-template"
	dbTimeZone           = "UTC"
	dbUsernameEnvVarName = "DB_USERNAME"
	dbPasswordEnvVarName = "DB_PASSWORD"

	// readinessTimeout bounds how long TestMain waits for each external
	// dependency to respond before exiting with a clear error.
	readinessTimeout = 5 * time.Second
)

// TestMain is the entry point for the test suite.
func TestMain(m *testing.M) {
	setupTestSuite()

	// Run the tests
	code := m.Run()

	// Teardown code can go here if needed
	tearDownTestSuite()

	// Exit with the test result code
	os.Exit(code)
}

func setupTestSuite() {
	fmt.Println("🧪 Setting up integration test environment")

	fmt.Print("⚙️  Setting up environment variables from file...")
	if err := config.SetEnvVarFromFile(); err != nil {
		fmt.Println("❌ Error setting environment variables from file:", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// Bring the dependency stack into focus before tests start so failures
	// surface here with a single clear message instead of as a hundred
	// opaque dial errors across the suite.
	setupTestDB()
	requireAPIReady()
	requireMailServerReady()

	fmt.Println("🧪 Setting integration test done... ✅")
}

func tearDownTestSuite() {
	fmt.Println("🔨 Tearing down integration test environment...")

	fmt.Print("🛢   Closing database connection pool...")
	testDBPool.Close()
	testDBPool = nil
	fmt.Println("✅")

	fmt.Print("🛢   Unsetting environment variables...")
	os.Unsetenv(dbUsernameEnvVarName)
	os.Unsetenv(dbPasswordEnvVarName)
	fmt.Println("✅")

	fmt.Print("🛢   Deleting all emails from the mail server...")
	if err := deleteAllEmails(); err != nil {
		fmt.Println("❌ Error deleting emails from mail server:", err)
	} else {
		fmt.Println("✅")
	}

	fmt.Println("🔨 Teardown integration test done... ✅")
}

func setupTestDB() {
	fmt.Print("🛢   Getting database user and password from environment variables...")
	dbUser := config.GetEnv(dbUsernameEnvVarName, "")
	dbPassword := config.GetEnv(dbPasswordEnvVarName, "")
	fmt.Println("✅")

	fmt.Print("🛢   Validating database user and password...")
	if dbUser == "" || dbPassword == "" {
		fmt.Println("❌ DB_USERNAME or DB_PASSWORD environment variable is not set or empty")
		os.Exit(1)
	}
	fmt.Println("✅")

	dbDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		dbHost,
		dbPort,
		dbUser,
		dbPassword,
		dbName,
		dbSSLMode,
		dbTimeZone,
	)

	fmt.Print("🛢   Parsing database connection string...")
	poolCfg, err := pgxpool.ParseConfig(dbDSN)
	if err != nil {
		fmt.Printf("❌ Failed to parse database config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	fmt.Print("🛢   Setting up database connection pool...")
	testDBPool, err = pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		fmt.Printf("❌ Failed to create database connection pool: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅")

	// pgxpool.NewWithConfig is lazy — it does not actually open a
	// connection. Ping with a bounded timeout so DB-down fails here
	// with one clear message instead of in every subtest.
	fmt.Print("🛢   Pinging database...")
	ctx, cancel := context.WithTimeout(context.Background(), readinessTimeout)
	defer cancel()
	if err := testDBPool.Ping(ctx); err != nil {
		fmt.Printf("❌ Database unreachable at %s:%d (%v).\n", dbHost, dbPort, err)
		fmt.Println("    Bring the dev env up first: make rm-dev-env && make start-dev-env && air")
		os.Exit(1)
	}
	fmt.Println("✅")
}

// requireAPIReady probes the API server's health endpoint and exits
// with a clear message if it is unreachable. The API server must be
// up before any test runs; without this probe, every HTTP test would
// fail with a generic connection-refused.
func requireAPIReady() {
	fmt.Printf("🌐  Probing API at %s...", apiHealthURL)
	ctx, cancel := context.WithTimeout(context.Background(), readinessTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiHealthURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to build API readiness request: %v\n", err)
		os.Exit(1)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ API unreachable at %s (%v).\n", apiHealthURL, err)
		fmt.Println("    Bring the dev env up first: make rm-dev-env && make start-dev-env && air")
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ API health returned status %d (expected 200) at %s\n", resp.StatusCode, apiHealthURL)
		os.Exit(1)
	}
	fmt.Println("✅")
}

// requireMailServerReady probes the mail server. Many tests assert on
// delivered emails, so the mail server being up is a hard requirement.
func requireMailServerReady() {
	probeURL := mailServerEndpointURL + "/messages"
	fmt.Printf("📬  Probing mail server at %s...", probeURL)
	ctx, cancel := context.WithTimeout(context.Background(), readinessTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		fmt.Printf("❌ Failed to build mail-server readiness request: %v\n", err)
		os.Exit(1)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Mail server unreachable at %s (%v).\n", probeURL, err)
		fmt.Println("    Bring the dev env up first: make rm-dev-env && make start-dev-env && air")
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("❌ Mail-server probe returned status %d (expected 200) at %s\n", resp.StatusCode, probeURL)
		os.Exit(1)
	}
	fmt.Println("✅")
}
