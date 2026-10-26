//go:build linux || darwin

package subproc

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// testdata/managed_git_off_golden.json was captured with captureGoldenGit on
// the commit this change is based on, before any isolation code existed.
func TestManagedGitOffMatchesBaselineGolden(t *testing.T) {
	setIsolationForTest(t, false)
	raw, err := os.ReadFile("testdata/managed_git_off_golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want map[string]goldenGit
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	if len(want) != 3 {
		t.Fatalf("golden holds %d helper classes, want interactive, lifecycle and background", len(want))
	}
	got := captureGoldenGit(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flag-off managed Git differs from the baseline\n got %+v\nwant %+v", got, want)
	}
}
