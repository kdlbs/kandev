package dynamic

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func TestIsFreeUsageQuota_PeriodLimitWithFreeNotice(t *testing.T) {
	free := Candidate{ModelID: "opencode/big-pickle", CatalogFree: true}
	classify := func(stderr string) *routingerr.Error {
		return routingerr.Classify(routingerr.Input{
			Phase: routingerr.PhasePromptSend, ProviderID: "opencode-acp", Stderr: stderr,
		})
	}
	withFree := classify("Error: daily usage limit reached\nError: free usage limit reached")
	if withFree.ClassifierRule != "opencode.stderr.usage_limit.v1" {
		t.Fatalf("classifier rule = %s, want the period usage-limit rule to win", withFree.ClassifierRule)
	}
	cases := []struct {
		name      string
		candidate Candidate
		failure   *routingerr.Error
		want      bool
	}{
		{"free notice on free Zen model", free, withFree, true},
		{"prefixed notice alone", free, classify("Error: free usage limit reached"), true},
		{"period limit without free notice", free, classify("Error: daily usage limit reached"), false},
		{"fenced free notice", free, classify("Error: daily usage limit reached\n```\nError: free usage limit reached\n```"), false},
		{"remote execution", Candidate{ModelID: free.ModelID, CatalogFree: true, RemoteExecution: true}, withFree, false},
		{"paid model", Candidate{ModelID: "opencode/paid-model"}, withFree, false},
		{"other provider model", Candidate{ModelID: "openrouter/some-model:free", CatalogFree: true}, withFree, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isFreeUsageQuota(tc.candidate, tc.failure); got != tc.want {
				t.Fatalf("isFreeUsageQuota = %v, want %v (rule %s)", got, tc.want, tc.failure.ClassifierRule)
			}
		})
	}
}
