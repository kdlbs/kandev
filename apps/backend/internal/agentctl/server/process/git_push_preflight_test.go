package process

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPushPreflightHistoryClassification(t *testing.T) {
	repoDir, originDir, _, binding, _, providerTwo, localHead := setupDivergedContributionRepo(t)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	operator.setRemoteContribution(binding)

	result, err := operator.PushPreflight(context.Background())
	if err != nil {
		t.Fatalf("PushPreflight returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("PushPreflight = %+v, want history-only rejection", result)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal preflight result: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatalf("decode preflight result: %v", err)
	}
	if got := body["preflight_reason"]; got != "history_update_required" {
		t.Fatalf("preflight_reason = %v, want history_update_required", got)
	}

	if got := strings.TrimSpace(runGit(t, repoDir, "rev-parse", "HEAD")); got != localHead {
		t.Fatalf("local HEAD changed during preflight: %q != %q", got, localHead)
	}
	if got := strings.TrimSpace(runGit(t, originDir, "rev-parse", "refs/heads/feature/contribution")); got != providerTwo {
		t.Fatalf("remote HEAD changed during preflight: %q != %q", got, providerTwo)
	}
}

func TestClassifyPushPreflightHistoryUpdate(t *testing.T) {
	const destination = "refs/heads/feature/remote"
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "fetch first",
			output: "! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (fetch first)",
			want:   true,
		},
		{
			name:   "non fast forward",
			output: "! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (non-fast-forward)",
			want:   true,
		},
		{
			name:   "wrong destination",
			output: "! HEAD:refs/heads/other refs/heads/other [rejected] (fetch first)",
		},
		{
			name:   "remote rejected",
			output: "! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (remote rejected)",
		},
		{
			name: "mixed failures",
			output: strings.Join([]string{
				"! HEAD:refs/heads/feature/remote refs/heads/feature/remote [rejected] (fetch first)",
				"! HEAD:refs/heads/other refs/heads/other [rejected] (remote rejected)",
			}, "\n"),
		},
		{
			name:   "malformed status",
			output: "! [rejected] (fetch first)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPushPreflightHistoryUpdate(tt.output, destination); got != tt.want {
				t.Fatalf("classifyPushPreflightHistoryUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}
