package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/sessionmcp"
)

func TestTokenVerifierComparesBrokerUpstream(t *testing.T) {
	recorded := sessionmcp.BrokerFingerprint(testBrokerURL)
	tests := []struct {
		name        string
		recorded    string
		advertised  string
		want        tokenVerdict
		wantDeletes int
	}{
		{"same upstream adopts", recorded, recorded, verdictAdopt, 0},
		{"no upstream on both sides adopts", "", "", verdictAdopt, 0},
		{"different upstream deletes", recorded, sessionmcp.BrokerFingerprint("http://other/resolve"), verdictDeleted, 1},
		{"relay appeared after create deletes", "", recorded, verdictDeleted, 1},
		{"relay missing deletes", recorded, "", verdictDeleted, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctl, v := verifyFixture(func() (*agentctl.InstanceIdentity, error) {
				return &agentctl.InstanceIdentity{
					Capabilities:              []string{"session_mcp_token"},
					MCPTokenFingerprint:       goodFP(),
					BrokerUpstreamFingerprint: tc.advertised,
				}, nil
			}, func() ([]byte, error) { return verifyKey(), nil }, "n1")
			v.BrokerFingerprint = tc.recorded
			if got := v.Verify(context.Background()); got != tc.want {
				t.Fatalf("verdict = %v, want %v", got, tc.want)
			}
			if len(ctl.deleted) != tc.wantDeletes {
				t.Fatalf("deletes = %q, want %d", ctl.deleted, tc.wantDeletes)
			}
			if tc.wantDeletes == 1 && ctl.deleted[0] != goodFP() {
				t.Fatalf("delete fingerprint = %q, want observed token fingerprint", ctl.deleted[0])
			}
		})
	}
}
