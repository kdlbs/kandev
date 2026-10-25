package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/secrets"
)

type fakeControl struct {
	deleted   []string // fingerprint carried by each delete
	deleteErr error
	revokes   int
	revokeErr []error // consumed per attempt; empty means success
}

func (f *fakeControl) DeleteInstanceMatching(_ context.Context, _, fp string) error {
	f.deleted = append(f.deleted, fp)
	return f.deleteErr
}

func (f *fakeControl) RevokeMCPToken(context.Context, string) error {
	f.revokes++
	if len(f.revokeErr) == 0 {
		return nil
	}
	err := f.revokeErr[0]
	f.revokeErr = f.revokeErr[1:]
	return err
}

func verifyKey() []byte { return []byte(strings.Repeat("k", sessionmcp.KeyLength)) }

func verifyFixture(identity func() (*agentctl.InstanceIdentity, error), key func() ([]byte, error), nonce string) (*fakeControl, tokenVerifier) {
	ctl := &fakeControl{}
	calls := 0
	return ctl, tokenVerifier{
		InstanceID: "inst-1",
		Nonce:      nonce,
		ReadKey:    key,
		Identity: func(context.Context) (*agentctl.InstanceIdentity, error) {
			calls++
			return identity()
		},
		Control:   ctl,
		Timeout:   time.Second,
		Attempts:  2,
		Gap:       time.Millisecond,
		Revokes:   3,
		RevokeGap: time.Millisecond,
	}
}

func goodFP() string { return sessionmcp.Fingerprint(sessionmcp.Token(verifyKey(), "inst-1", "n1")) }

func TestTokenVerifierOutcomes(t *testing.T) {
	okKey := func() ([]byte, error) { return verifyKey(), nil }
	serve := func(fp string) func() (*agentctl.InstanceIdentity, error) {
		return func() (*agentctl.InstanceIdentity, error) {
			return &agentctl.InstanceIdentity{Capabilities: []string{"session_mcp_token"}, MCPTokenFingerprint: fp}, nil
		}
	}
	fail := func(err error) func() (*agentctl.InstanceIdentity, error) {
		return func() (*agentctl.InstanceIdentity, error) { return nil, err }
	}
	tests := []struct {
		name        string
		identity    func() (*agentctl.InstanceIdentity, error)
		key         func() ([]byte, error)
		nonce       string
		want        tokenVerdict
		wantDeleted []string
		wantRevokes int
	}{
		{"match adopts", serve(goodFP()), okKey, "n1", verdictAdopt, nil, 0},
		{"mismatch deletes with observed fingerprint", serve("other-fp"), okKey, "n1", verdictDeleted, []string{"other-fp"}, 0},
		{"revoked instance deletes unconditionally", serve(""), okKey, "n1", verdictDeleted, []string{""}, 0},
		{"missing nonce deletes", serve(goodFP()), okKey, "", verdictDeleted, []string{goodFP()}, 0},
		{"malformed body deletes unconditionally", fail(agentctl.ErrIdentityMalformed), okKey, "n1", verdictDeleted, []string{""}, 0},
		{"key not found deletes", serve(goodFP()), func() ([]byte, error) { return nil, secrets.ErrNotFound }, "n1", verdictDeleted, []string{goodFP()}, 0},
		{"key unusable deletes", serve(goodFP()), func() ([]byte, error) { return nil, secrets.ErrSessionKeyUnusable }, "n1", verdictDeleted, []string{goodFP()}, 0},
		{"key store failure revokes", serve(goodFP()), func() ([]byte, error) { return nil, errors.New("disk") }, "n1", verdictUnknown, nil, 1},
		{"404 is absent", fail(&agentctl.StatusError{Op: "identity", Status: 404}), okKey, "n1", verdictAbsent, nil, 0},
		{"401 is unknown and revokes", fail(&agentctl.StatusError{Op: "identity", Status: 401}), okKey, "n1", verdictUnknown, nil, 1},
		{"403 is unknown and revokes", fail(&agentctl.StatusError{Op: "identity", Status: 403}), okKey, "n1", verdictUnknown, nil, 1},
		{"5xx exhausts attempts then revokes", fail(&agentctl.StatusError{Op: "identity", Status: 503}), okKey, "n1", verdictUnknown, nil, 1},
		{"transport error is unknown", fail(errors.New("refused")), okKey, "n1", verdictUnknown, nil, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctl, v := verifyFixture(tc.identity, tc.key, tc.nonce)
			got := v.Verify(context.Background())
			if got != tc.want {
				t.Fatalf("verdict = %v, want %v", got, tc.want)
			}
			if strings.Join(ctl.deleted, ",") != strings.Join(tc.wantDeleted, ",") {
				t.Fatalf("deletes = %q, want %q", ctl.deleted, tc.wantDeleted)
			}
			if ctl.revokes != tc.wantRevokes {
				t.Fatalf("revokes = %d, want %d", ctl.revokes, tc.wantRevokes)
			}
		})
	}
}

func TestTokenVerifierRetriesTransientReadThenAdopts(t *testing.T) {
	n := 0
	ctl, v := verifyFixture(func() (*agentctl.InstanceIdentity, error) {
		n++
		if n == 1 {
			return nil, errors.New("timeout")
		}
		return &agentctl.InstanceIdentity{MCPTokenFingerprint: goodFP()}, nil
	}, func() ([]byte, error) { return verifyKey(), nil }, "n1")
	if got := v.Verify(context.Background()); got != verdictAdopt || len(ctl.deleted) != 0 || ctl.revokes != 0 {
		t.Fatalf("verdict=%v deletes=%v revokes=%d", got, ctl.deleted, ctl.revokes)
	}
}

func TestTokenVerifierRevokeRepeatsThreeTimesThenGivesUp(t *testing.T) {
	ctl, v := verifyFixture(func() (*agentctl.InstanceIdentity, error) { return nil, errors.New("refused") },
		func() ([]byte, error) { return verifyKey(), nil }, "n1")
	ctl.revokeErr = []error{errors.New("x"), errors.New("x"), errors.New("x"), errors.New("x")}
	if got := v.Verify(context.Background()); got != verdictUnknown || ctl.revokes != 3 {
		t.Fatalf("verdict=%v revokes=%d, want unknown after 3", got, ctl.revokes)
	}
}

func TestTokenVerifierCancelledContextIsUnknownAndStillRevokes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctl, v := verifyFixture(func() (*agentctl.InstanceIdentity, error) { return nil, ctx.Err() },
		func() ([]byte, error) { return verifyKey(), nil }, "n1")
	if got := v.Verify(ctx); got != verdictUnknown || ctl.revokes != 1 {
		t.Fatalf("verdict=%v revokes=%d", got, ctl.revokes)
	}
}

func TestTokenVerifierNilControlStillReportsUnknown(t *testing.T) {
	_, v := verifyFixture(func() (*agentctl.InstanceIdentity, error) { return nil, errors.New("x") },
		func() ([]byte, error) { return verifyKey(), nil }, "n1")
	v.Control = nil
	if got := v.Verify(context.Background()); got != verdictUnknown {
		t.Fatalf("verdict=%v", got)
	}
}
