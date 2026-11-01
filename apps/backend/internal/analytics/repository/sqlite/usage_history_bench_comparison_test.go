//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
)

type usageBenchComparisonReceipt struct {
	SelectionDigest string               `json:"selection_sha256"`
	Publication     string               `json:"publication"`
	Scans           usageBenchScanTotals `json:"scans"`
	P95NS           int64                `json:"p95_ns"`
	Samples         int                  `json:"latency_sample_count"`
}

func usageBenchSelectionDigest(t testing.TB, result usageBenchResult) string {
	t.Helper()
	first := result.Pages[0]
	selection := struct {
		Rows      []models.TokenUsageRow
		Totals    models.UsageTotals
		Providers []string
	}{result.Rows, first.Totals, first.Providers}
	encoded, err := json.Marshal(selection)
	usageBenchMust(t, err)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func writeUsageBenchComparison(t testing.TB, engine string, before, after map[string]usageBenchComparisonReceipt) {
	t.Helper()
	type comparison struct {
		Before usageBenchComparisonReceipt `json:"one_year"`
		After  usageBenchComparisonReceipt `json:"five_years"`
	}
	comparisons := map[string]comparison{}
	for name, first := range before {
		if !strings.HasPrefix(name, "recent/") {
			continue
		}
		last, ok := after[name]
		if !ok || first.SelectionDigest != last.SelectionDigest || first.Publication == last.Publication {
			t.Fatalf("%s recent report changed or did not publish after four older years", name)
		}
		comparisons[name] = comparison{Before: first, After: last}
	}
	if len(comparisons) != 4 {
		t.Fatal("benchmark did not measure both shapes for all four recent calendars")
	}
	encoded, err := json.MarshalIndent(comparisons, "", "  ")
	usageBenchMust(t, err)
	path := filepath.Join(os.Getenv("KANDEV_USAGE_BENCH_EVIDENCE_DIR"), usageBenchReceiptPrefix+engine+"-recent-before-after.json")
	usageBenchMust(t, os.WriteFile(path, append(encoded, '\n'), 0o644))
}
