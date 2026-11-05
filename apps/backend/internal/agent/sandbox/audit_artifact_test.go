package sandbox

import (
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestFinalAuditReceiptSQLite(t *testing.T) { runFinalAuditReceiptStore(t, sqlitePolicyDB(t)) }
func TestFinalAuditReceiptPostgres(t *testing.T) {
	runFinalAuditReceiptStore(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func runFinalAuditReceiptStore(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database, 2)
	ctx := operatorContext(t)
	var err error
	attempt, err = service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	binding, err := AuditBindingDigest(attempt)
	require.NoError(t, err)
	data := reportFixture()
	data.Finalized = true
	data.FinalAudit = &FinalAuditReceipt{BindingDigest: binding, ArtifactSetDigest: strings.Repeat("d", 64), CapturedAt: time.Now().UTC()}
	for _, invalid := range []string{"foreign-binding", "future", "not-finalized", "invalid-digest"} {
		changed := data
		receipt := *data.FinalAudit
		changed.FinalAudit = &receipt
		switch invalid {
		case "foreign-binding":
			receipt.BindingDigest = strings.Repeat("e", 64)
		case "future":
			receipt.CapturedAt = time.Now().Add(time.Hour)
		case "not-finalized":
			changed.Finalized = false
		case "invalid-digest":
			receipt.ArtifactSetDigest = "untrusted text"
		}
		_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, changed)
		require.Error(t, err, invalid)
	}
	stored, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, data)
	require.NoError(t, err)
	restarted, err := NewService(database, true, nil)
	require.NoError(t, err)
	read, err := restarted.ReadCapturedReport(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, stored, read)
	foreign := *attempt
	foreign.ID = "another-attempt"
	foreignBinding, err := AuditBindingDigest(&foreign)
	require.NoError(t, err)
	require.NotEqual(t, binding, foreignBinding)
	foreign = *attempt
	owner := *attempt.Owner
	owner.ProviderSessionID = "another-session"
	foreign.Owner = &owner
	foreignBinding, err = AuditBindingDigest(&foreign)
	require.NoError(t, err)
	require.NotEqual(t, binding, foreignBinding)
}
