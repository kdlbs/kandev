//go:build linux

package sandboxruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func assertRuntimeReportSurvivesInvalidAudit(t *testing.T, ctx context.Context, prepared *Prepared, previous *policy.Report) {
	t.Helper()
	for _, source := range prepared.description.RuntimeSources {
		if source.Kind != "audit" {
			continue
		}
		files, err := contract.ReadRuntimeSource(ctx, prepared.files.Directories.State, contract.RuntimeSource{
			Key: source.Key, Kind: source.Kind, Directory: source.RelativeDirectory, Suffix: source.FilenameSuffix,
			MaxDepth: source.MaxDepth, MaxFiles: source.MaxFiles, MaxBytes: min(source.MaxBytes, 1<<20)})
		require.NoError(t, err)
		require.NotEmpty(t, files)
		for _, file := range files {
			path := filepath.Join(prepared.files.Directories.State, file.Name)
			require.NoError(t, os.WriteFile(path, []byte("invalid audit record\n"), 0o600))
			t.Cleanup(func() { _ = os.WriteFile(path, file.Content, 0o600) })
		}
		report, err := prepared.manager.RefreshReport(ctx, prepared.orgID, prepared.launch.AttemptID)
		require.NoError(t, err)
		require.Equal(t, "unavailable", report.Data.Status)
		require.False(t, report.Data.Complete)
		require.Equal(t, previous.Data.Records, report.Data.Records)
		require.Equal(t, previous.Data.Sources, report.Data.Sources)
		require.Equal(t, previous.Data.V2, report.Data.V2)
		require.Equal(t, previous.Data.LastSuccessfulReadAt, report.Data.LastSuccessfulReadAt)
		require.NotEmpty(t, report.Data.RefreshError)
		for _, file := range files {
			require.NoError(t, os.WriteFile(filepath.Join(prepared.files.Directories.State, file.Name), file.Content, 0o600))
		}
	}
}

func assertRuntimeFinalArchive(t *testing.T, ctx context.Context, prepared *Prepared, report *policy.Report) string {
	t.Helper()
	require.NotNil(t, report.Data.FinalAudit, "final bytes must survive runtime cleanup independently from normalized rows")
	receipt := report.Data.FinalAudit
	if report.Data.Status == "available" {
		require.Equal(t, report.Data.V2.ArtifactSetDigest, receipt.ArtifactSetDigest)
	}
	attempt, err := prepared.manager.policies.ReadAttempt(ctx, prepared.orgID, prepared.launch.AttemptID)
	require.NoError(t, err)
	binding, err := policy.AuditBindingDigest(attempt)
	require.NoError(t, err)
	require.Equal(t, binding, receipt.BindingDigest)
	base := filepath.Join(prepared.manager.base, "audit-archives")
	key := reportHash([]byte(binding + "\x00" + receipt.ArtifactSetDigest))
	archive, err := contract.LoadAuditArchive(ctx, base, base, key, binding, receipt.ArtifactSetDigest)
	require.NoError(t, err)
	defer func() { require.NoError(t, archive.Close()) }()
	var metadata bool
	for _, source := range archive.Sources {
		if strings.HasSuffix(source.Name, "/session.json") {
			metadata = true
		}
	}
	require.True(t, metadata, "retained artifacts must include finalized metadata")
	require.NoDirExists(t, prepared.files.Root, "the retained audit must not keep the runtime allocation alive")
	return filepath.Join(base, key)
}

func assertRuntimeDenials(t *testing.T, report *policy.Report, target string, finalized bool) {
	t.Helper()
	require.Equal(t, "available", report.Data.Status)
	require.Equal(t, "partial", report.Data.Integrity)
	require.False(t, report.Data.Complete, "capability audit does not contain every filesystem or network denial")
	require.Equal(t, finalized, report.Data.Finalized)
	require.NotEmpty(t, report.Data.Sources)
	require.NotNil(t, report.Data.LastSuccessfulReadAt)
	require.NotNil(t, report.Data.V2, "major 2 typed report capability must select the streaming reader")
	require.Equal(t, "valid", report.Data.V2.Integrity)
	require.Len(t, report.Data.V2.Coverage, 4)
	require.NotEmpty(t, report.Data.V2.Records)
	if finalized {
		require.Equal(t, "final", report.Data.V2.Finalization)
	}
	var found bool
	for _, record := range report.Data.Records {
		if record.Kind == "filesystem" && record.Operation == "read" && record.Target == target {
			found = true
		}
	}
	require.True(t, found, "retain the explicit denied read")
}
