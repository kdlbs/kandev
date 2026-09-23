package gitbase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type gitFake struct {
	commands [][]string
	outputs  map[string]string
	errors   map[string]error
}

func (f *gitFake) run(_ context.Context, args ...string) (string, error) {
	f.commands = append(f.commands, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if err := f.errors[key]; err != nil {
		return "", err
	}
	if output, ok := f.outputs[key]; ok {
		return output, nil
	}
	if len(args) >= 3 && args[0] == "config" && args[1] == "--get" && strings.HasPrefix(args[2], "remote.") {
		return "", errors.New("not configured")
	}
	return "", nil
}

func testPRBase() models.PRBase {
	return models.PRBase{Target: models.ComparisonTarget{
		Version:      models.ComparisonTargetVersion,
		Provider:     models.ComparisonTargetProviderGitHub,
		Kind:         models.ComparisonTargetKindPullRequest,
		Number:       42,
		HeadBranch:   "feature/same-name",
		TargetBranch: "release/next",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork/widget", ProviderID: "fork-1",
			RemoteURL: "https://github.com/fork/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", ProviderID: "base-1",
			RemoteURL: "https://github.com/upstream/widget.git",
		},
	}}
}

func TestMaterializeQualifiedBaseFetchesExactTargetAndVerifiesOID(t *testing.T) {
	base := testPRBase()
	base.OID = "0123456789abcdef0123456789abcdef01234567"
	fake := &gitFake{outputs: map[string]string{
		"rev-parse --verify " + base.Target.ComparisonRef() + "^{commit}": base.OID,
	}, errors: map[string]error{}}
	got, err := Materialize(context.Background(), fake.run, base)
	if err != nil {
		t.Fatalf("Materialize(): %v", err)
	}
	if got.Ref != base.Target.ComparisonRef() || got.OID != base.OID {
		t.Fatalf("materialization = %#v, want qualified ref and observed OID", got)
	}
	wantFetch := []string{"fetch", "--no-tags", base.Target.ComparisonRemoteName(), "+refs/heads/release/next:" + base.Target.ComparisonRef()}
	if !hasCommand(fake.commands, wantFetch...) {
		t.Fatalf("fetch commands = %#v, missing %#v", fake.commands, wantFetch)
	}
	if hasCommand(fake.commands, "fetch", "--no-tags", "origin", "+refs/heads/release/next:release/next") {
		t.Fatalf("materialized an unqualified origin branch: %#v", fake.commands)
	}
}

func TestMaterializeQualifiedBaseRejectsOIDDrift(t *testing.T) {
	base := testPRBase()
	base.OID = "0123456789abcdef0123456789abcdef01234567"
	fake := &gitFake{outputs: map[string]string{
		"rev-parse --verify " + base.Target.ComparisonRef() + "^{commit}": "fedcba9876543210fedcba9876543210fedcba98",
	}, errors: map[string]error{}}
	_, err := Materialize(context.Background(), fake.run, base)
	if ErrorCode(err) != ErrorOIDMismatch {
		t.Fatalf("ErrorCode() = %q, want %q (err=%v)", ErrorCode(err), ErrorOIDMismatch, err)
	}
}

func TestMaterializeQualifiedBaseRejectsRemoteCollision(t *testing.T) {
	base := testPRBase()
	fake := &gitFake{
		outputs: map[string]string{"config --get remote." + base.Target.ComparisonRemoteName() + ".url": "https://github.com/other/widget.git"},
		errors:  map[string]error{},
	}
	_, err := Materialize(context.Background(), fake.run, base)
	if ErrorCode(err) != ErrorRemoteCollision {
		t.Fatalf("ErrorCode() = %q, want %q (err=%v)", ErrorCode(err), ErrorRemoteCollision, err)
	}
}

func TestMaterializeQualifiedBasePropagatesFetchAndAuthenticationErrors(t *testing.T) {
	base := testPRBase()
	remoteName := base.Target.ComparisonRemoteName()
	authErr := errors.New("authentication failed")
	fake := &gitFake{
		errors: map[string]error{
			"fetch --no-tags " + remoteName + " +refs/heads/release/next:" + base.Target.ComparisonRef(): authErr,
		},
		outputs: map[string]string{},
	}
	_, err := Materialize(context.Background(), fake.run, base)
	if ErrorCode(err) != ErrorFetch || !errors.Is(err, authErr) {
		t.Fatalf("Materialize() error = %v, code = %q, want propagated auth failure", err, ErrorCode(err))
	}
}

func TestMaterializeQualifiedBasePreservesCancellation(t *testing.T) {
	base := testPRBase()
	canceled := context.Canceled
	fake := &gitFake{errors: map[string]error{
		"remote add --no-tags " + base.Target.ComparisonRemoteName() + " " + base.Target.TargetRepository.RemoteURL: canceled,
	}, outputs: map[string]string{}}
	_, err := Materialize(context.Background(), fake.run, base)
	if !errors.Is(err, canceled) {
		t.Fatalf("Materialize() error = %v, want context.Canceled", err)
	}
}

func TestMaterializeQualifiedBaseFetchesPRHeadFromBaseRepository(t *testing.T) {
	target := testPRBase().Target
	fake := &gitFake{outputs: map[string]string{}, errors: map[string]error{}}
	ref, err := FetchPullRequestHead(context.Background(), fake.run, target)
	if err != nil {
		t.Fatalf("FetchPullRequestHead(): %v", err)
	}
	wantRef := "refs/remotes/" + target.ComparisonRemoteName() + "/pull/42/head"
	if ref != wantRef || !hasCommand(fake.commands, "fetch", "--no-tags", target.ComparisonRemoteName(), "+refs/pull/42/head:"+wantRef) {
		t.Fatalf("head ref = %q, commands = %#v", ref, fake.commands)
	}
}

func hasCommand(commands [][]string, want ...string) bool {
	for _, command := range commands {
		if len(command) != len(want) {
			continue
		}
		match := true
		for i := range want {
			if command[i] != want[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
