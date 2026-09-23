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

type gitCommandError struct {
	code int
}

func (e gitCommandError) Error() string { return "git command failed" }

func (e gitCommandError) ExitCode() int { return e.code }

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
		return "", gitCommandError{code: 1}
	}
	return "", nil
}

func TestMaterializeQualifiedBaseDoesNotReplaceRemoteAfterConfigReadFailure(t *testing.T) {
	base := testPRBase()
	remoteName := base.Target.ComparisonRemoteName()
	readErr := errors.New("repository config is unreadable")
	fake := &gitFake{
		errors:  map[string]error{"config --get remote." + remoteName + ".url": readErr},
		outputs: map[string]string{},
	}
	_, err := Materialize(context.Background(), fake.run, base)
	if ErrorCode(err) != ErrorRemoteSetup || !errors.Is(err, readErr) {
		t.Fatalf("Materialize() error = %v, code = %q, want preserved config-read failure", err, ErrorCode(err))
	}
	if hasCommand(fake.commands, "remote", "add", "--no-tags", remoteName, base.Target.TargetRepository.RemoteURL) {
		t.Fatalf("Materialize() replaced remote after config-read failure: %#v", fake.commands)
	}
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
	headOID := "fedcba9876543210fedcba9876543210fedcba98"
	wantRef := "refs/remotes/" + target.ComparisonRemoteName() + "/pull/42/head"
	fake := &gitFake{outputs: map[string]string{
		"rev-parse --verify " + wantRef + "^{commit}": headOID,
	}, errors: map[string]error{}}
	head, err := FetchPullRequestHead(context.Background(), fake.run, target)
	if err != nil {
		t.Fatalf("FetchPullRequestHead(): %v", err)
	}
	if head.Ref != wantRef || head.OID != headOID ||
		!hasCommand(fake.commands, "fetch", "--no-tags", target.ComparisonRemoteName(), "+refs/pull/42/head:"+wantRef) {
		t.Fatalf("head snapshot = %#v, commands = %#v", head, fake.commands)
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
