package backendapp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresBootInitializesRepositories(t *testing.T) {
	adminDSN := testutil.PostgresDSNFromEnv(t)
	adminInfo, err := parsePostgresConnInfo(adminDSN)
	if err != nil {
		t.Fatalf("parse postgres dsn: %v", err)
	}
	admin := openPostgresAdmin(t, adminDSN)

	dbName := "kandev_boot_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("create postgres database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)")
	})

	cfg := &config.Config{
		HomeDir: t.TempDir(),
		Database: config.DatabaseConfig{
			Driver:   "postgres",
			Host:     adminInfo.Host,
			Port:     adminInfo.Port,
			User:     adminInfo.User,
			Password: adminInfo.Password,
			DBName:   dbName,
			SSLMode:  adminInfo.SSLMode,
			MaxConns: 5,
			MinConns: 1,
		},
	}
	log := newTestLogger()

	pool, repos, cleanups, err := provideRepositories(context.Background(), cfg, log, "test-postgres-boot")
	if err != nil {
		t.Fatalf("provide repositories with postgres: %v", err)
	}
	if pool == nil || repos == nil {
		t.Fatal("provideRepositories returned nil pool or repositories")
	}
	t.Cleanup(func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				_ = cleanups[i]()
			}
		}
	})

	agentRegistry, registryCleanup, err := registry.Provide(log)
	if err != nil {
		t.Fatalf("provide agent registry: %v", err)
	}
	t.Cleanup(func() {
		if registryCleanup != nil {
			_ = registryCleanup()
		}
	})
	services, agentSettingsController, err := provideServices(
		cfg,
		log,
		repos,
		pool,
		bus.NewMemoryEventBus(log),
		agentRegistry,
		"test-postgres-boot",
	)
	if err != nil {
		t.Fatalf("provide services with postgres: %v", err)
	}
	if services.GitHub == nil {
		t.Fatal("GitHub service is unavailable after PostgreSQL boot")
	}
	if _, err := services.GitHub.GetWorkspaceSettings(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("GitHub persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.GitLab == nil {
		t.Fatal("GitLab service is unavailable after PostgreSQL boot")
	}
	if _, err := services.GitLab.GetConfigForWorkspace(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("GitLab task persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.Jira == nil {
		t.Fatal("Jira service is unavailable after PostgreSQL boot")
	}
	if _, err := services.Jira.GetConfigForWorkspace(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("Jira persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.Linear == nil {
		t.Fatal("Linear service is unavailable after PostgreSQL boot")
	}
	if _, err := services.Linear.GetConfigForWorkspace(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("Linear persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.Sentry == nil || services.Sentry.Store() == nil {
		t.Fatal("Sentry persistence is unavailable after PostgreSQL boot")
	}
	if _, err := services.Sentry.ListInstances(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("Sentry persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.AzureDevOps == nil {
		t.Fatal("Azure DevOps service is unavailable after PostgreSQL boot")
	}
	if _, err := services.AzureDevOps.GetConfigForWorkspace(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("Azure DevOps persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.WorkflowSync == nil {
		t.Fatal("workflow sync service is unavailable after PostgreSQL boot")
	}
	if _, err := services.WorkflowSync.GetConfigForWorkspace(context.Background(), "boot-workspace"); err != nil {
		t.Fatalf("workflow sync persistence is unavailable after PostgreSQL boot: %v", err)
	}
	if services.Automation == nil || services.Automation.Service == nil {
		t.Fatal("automation persistence is unavailable after PostgreSQL boot")
	}
	if err := runInitialAgentSetup(context.Background(), services.User, agentSettingsController, log); err != nil {
		t.Fatalf("run initial agent setup with postgres: %v", err)
	}
}

type postgresConnInfo struct {
	Host     string
	Port     int
	User     string
	Password string
	SSLMode  string
}

func openPostgresAdmin(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres admin connection: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := db.Ping(); err != nil {
		t.Fatalf("ping postgres admin connection: %v", err)
	}
	return db
}

func parsePostgresConnInfo(dsn string) (postgresConnInfo, error) {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		return postgresConnInfo{}, err
	}
	info := postgresConnInfo{
		Host:     cfg.Host,
		Port:     int(cfg.Port),
		User:     cfg.User,
		Password: cfg.Password,
		SSLMode:  postgresSSLMode(dsn),
	}
	if info.Host == "" {
		info.Host = "localhost"
	}
	if info.Port == 0 {
		info.Port = 5432
	}
	return info, nil
}

func postgresSSLMode(dsn string) string {
	if parsed, err := url.Parse(dsn); err == nil && parsed.Scheme != "" {
		if mode := parsed.Query().Get("sslmode"); mode != "" {
			return mode
		}
	}
	for _, field := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(field, "=")
		if ok && strings.EqualFold(key, "sslmode") {
			return strings.Trim(value, "'\"")
		}
	}
	if mode := os.Getenv("PGSSLMODE"); mode != "" {
		return mode
	}
	return "disable"
}

func TestParsePostgresConnInfo(t *testing.T) {
	info, err := parsePostgresConnInfo(
		"host=postgres port=5433 user=kandev password=secret dbname=kandev_test sslmode=require",
	)
	if err != nil {
		t.Fatalf("parse conn info: %v", err)
	}
	want := postgresConnInfo{
		Host:     "postgres",
		Port:     5433,
		User:     kandevName,
		Password: "secret",
		SSLMode:  "require",
	}
	if info != want {
		t.Fatalf("conn info = %s, want %s", formatConnInfo(info), formatConnInfo(want))
	}
}

func formatConnInfo(info postgresConnInfo) string {
	return fmt.Sprintf("{host:%s port:%d user:%s sslmode:%s}", info.Host, info.Port, info.User, info.SSLMode)
}
