package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func reportFixture() ReportData {
	return ReportData{Status: "available", Integrity: "partial", SupportedClasses: []string{"filesystem"},
		Sources: []ReportSource{{ID: "source", Schema: "fixture.audit.v1", Digest: strings.Repeat("a", 64), Integrity: "partial"}},
		Records: []DenialRecord{{ID: "denial", SourceID: "source", ProviderRecordID: "request", Kind: "filesystem", Operation: "read", Target: "/workspace/denied", OccurredAt: time.Now().UTC().Truncate(time.Millisecond)}},
	}
}

func TestSandboxReportStoreSQLite(t *testing.T) { testSandboxReportStore(t, sqlitePolicyDB(t)) }
func TestSandboxReportStorePostgres(t *testing.T) {
	testSandboxReportStore(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

// AC-AGENTS-SANDBOX-004.1: retain bounded typed denials and their incomplete source.
func testSandboxReportStore(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	data := reportFixture()
	start := make(chan struct{})
	type outcome struct {
		report *Report
		err    error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			report, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, data)
			results <- outcome{report, err}
		}()
	}
	close(start)
	var report *Report
	for range 2 {
		result := <-results
		if result.err != nil {
			require.ErrorIs(t, result.err, ErrConflict)
		} else {
			require.Nil(t, report, "exactly one report capture wins")
			report = result.report
		}
	}
	require.NotNil(t, report)
	require.Equal(t, int64(1), report.Version)
	require.Equal(t, attempt.Snapshot.Resolved.Revision.ID, report.RevisionID)
	require.Equal(t, attempt.Snapshot.Resolved.Ceiling.Digest, report.CeilingDigest)
	require.Equal(t, data, report.Data)
	require.False(t, report.Data.Complete)

	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	reloaded.SetSessionAuthorizer(func(context.Context, string) error { return nil })
	read, err := reloaded.ReadReport(ctx, "session", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, report, read)

	for _, caller := range []context.Context{
		context.Background(),
		authn.WithIdentity(t.Context(), authn.Identity{UserID: "foreign", SessionID: "login", OrgID: "other", Role: authn.RoleAdmin}),
		authn.WithIdentity(t.Context(), authn.Identity{UserID: "token", OrgID: "org", TokenID: "pat", Role: authn.RoleAdmin}),
	} {
		_, err = reloaded.ReadReport(caller, "session", attempt.ID)
		require.Error(t, err)
	}
	_, err = service.ReadReport(ctx, "foreign-session", attempt.ID)
	require.Error(t, err)
	_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, data)
	require.ErrorIs(t, err, ErrConflict, "stale report readers cannot overwrite a newer capture")
	_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version-1, report.Version, data)
	require.ErrorIs(t, err, ErrConflict, "old runtime owners cannot publish")
	data.Finalized = true
	_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, report.Version, data)
	require.Error(t, err, "a live process cannot produce a finalized report")
	attempt, err = service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	final, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, report.Version, data)
	require.NoError(t, err)
	require.True(t, final.Data.Finalized)
	data.Finalized = false
	_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, final.Version, data)
	require.ErrorIs(t, err, ErrConflict, "a completed capture cannot be replaced by an old live read")
	read, err = service.ReadReport(ctx, "session", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, final, read)
	unchanged, err := service.ReadAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, attempt.Snapshot, unchanged.Snapshot)
}

func TestSandboxReportUnavailableAndCorruptStorage(t *testing.T) {
	database := sqlitePolicyDB(t)
	service, attempt, _ := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	data := ReportData{Status: "unavailable", Integrity: "unverified"}
	report, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, data)
	require.NoError(t, err)
	require.False(t, report.Data.Complete)
	require.Empty(t, report.Data.Records)
	_, err = database.Exec(`UPDATE sandbox_reports SET content_json=replace(content_json,'unverified','complete')`)
	require.NoError(t, err)
	_, err = service.ReadReport(ctx, "session", attempt.ID)
	require.ErrorIs(t, err, ErrConflict)
}

func TestSandboxReportRejectsInvalidEvidence(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
	for name, change := range map[string]func(*ReportData){
		"invented class":             func(data *ReportData) { data.Records[0].Kind = "launch_error" },
		"unknown source":             func(data *ReportData) { data.Records[0].SourceID = "foreign" },
		"duplicate denial":           func(data *ReportData) { data.Records = append(data.Records, data.Records[0]) },
		"missing source digest":      func(data *ReportData) { data.Sources[0].Digest = "" },
		"false completeness":         func(data *ReportData) { data.Complete = true },
		"private control characters": func(data *ReportData) { data.Records[0].Target = "/file\x00secret" },
		"oversized target":           func(data *ReportData) { data.Records[0].Target = strings.Repeat("x", 4097) },
		"unsupported record":         func(data *ReportData) { data.Status = "unsupported" },
	} {
		t.Run(name, func(t *testing.T) {
			data := reportFixture()
			change(&data)
			_, err := service.CaptureReport(t.Context(), "org", attempt.ID, attempt.Version, 0, data)
			require.Error(t, err)
		})
	}
	_, err := service.CaptureReport(t.Context(), "org", attempt.ID, attempt.Version, 0, reportFixture())
	require.NoError(t, err, "valid evidence remains reachable after rejected writes")
}
