package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/org"
)

var orgCredentialSequence atomic.Uint64

func TestOrganizationCredentialLookupWiring(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:org-credential-%d?mode=memory&cache=shared", orgCredentialSequence.Add(1))
	writer, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	reader, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })

	pool := db.NewPool(writer, reader)
	store, err := org.NewStore(pool)
	if err != nil {
		t.Fatalf("org store: %v", err)
	}
	service := org.NewService(store, nil, nil, true, logger.Default())
	checker := organizationStatusChecker(service)

	active, err := store.Create(ctx, "Acme", "acme", true)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if usable, err := checker(ctx, active.ID); err != nil || !usable {
		t.Fatalf("active organization = %v, %v; want true, nil", usable, err)
	}

	missingOK, missingErr := checker(ctx, "missing-org")
	if missingOK || missingErr != nil {
		t.Fatalf("missing organization = %v, %v; want false, nil", missingOK, missingErr)
	}

	if _, err := store.UpdateNameStatus(ctx, active.ID, "", string(org.StatusSuspended)); err != nil {
		t.Fatalf("suspend org: %v", err)
	}
	if usable, err := checker(ctx, active.ID); err != nil || usable {
		t.Fatalf("suspended organization = %v, %v; want false, nil", usable, err)
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	readOK, readErr := checker(ctx, active.ID)
	if readOK || readErr == nil {
		t.Fatalf("unreadable organization = %v, %v; want false, error", readOK, readErr)
	}
	if errors.Is(readErr, org.ErrOrgNotFound) {
		t.Fatalf("read failure must not be classified as a missing organization: %v", readErr)
	}
}
