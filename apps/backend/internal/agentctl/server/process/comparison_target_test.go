package process

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type comparisonTargetGitFake struct {
	remoteURL string
	commands  [][]string
	outputs   map[string]string
	errors    map[string]error
}

func (f *comparisonTargetGitFake) run(_ context.Context, args ...string) (string, error) {
	f.commands = append(f.commands, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if err := f.errors[key]; err != nil {
		return "", err
	}
	if output, ok := f.outputs[key]; ok {
		return output, nil
	}
	if len(args) >= 3 && args[0] == "remote" && args[1] == "get-url" {
		if f.remoteURL == "" {
			return "", errors.New("remote not found")
		}
		return f.remoteURL, nil
	}
	return "", nil
}

func comparisonTargetProcessTestTarget() models.ComparisonTarget {
	return models.ComparisonTarget{
		Version:      models.ComparisonTargetVersion,
		Provider:     models.ComparisonTargetProviderGitHub,
		Kind:         models.ComparisonTargetKindPullRequest,
		Number:       1154,
		HeadBranch:   "feature/cursor-cost",
		TargetBranch: "main",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "contributor/widget", ProviderID: "head-42",
			RemoteURL: "https://github.com/contributor/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widget", ProviderID: "base-99",
			RemoteURL: "https://github.com/upstream/widget.git",
		},
	}
}

func TestMaterializeComparisonTargetUsesExactNoPushRefspec(t *testing.T) {
	target := comparisonTargetProcessTestTarget()
	fake := &comparisonTargetGitFake{
		outputs: map[string]string{
			"rev-parse --verify " + target.ComparisonRef() + "^{commit}": "0123456789abcdef0123456789abcdef01234567",
		},
		errors: map[string]error{},
	}
	state, err := materializeComparisonTarget(context.Background(), fake.run, target)
	if err != nil {
		t.Fatalf("materializeComparisonTarget: %v", err)
	}
	if state.Ref != target.ComparisonRef() || state.RemoteName != target.ComparisonRemoteName() {
		t.Fatalf("state = %#v, want target ref and remote", state)
	}
	if !hasComparisonCommand(fake.commands, "remote", "add", "--no-tags", target.ComparisonRemoteName(), target.TargetRepository.RemoteURL) {
		t.Fatalf("remote add command missing: %v", fake.commands)
	}
	if !hasComparisonCommand(fake.commands, "config", "remote."+target.ComparisonRemoteName()+".pushurl", "DISABLED") {
		t.Fatalf("push disabling command missing: %v", fake.commands)
	}
	if !hasComparisonCommand(fake.commands, "fetch", "--no-tags", target.ComparisonRemoteName(), "refs/heads/main:"+target.ComparisonRef()) {
		t.Fatalf("exact fetch command missing: %v", fake.commands)
	}
	for _, command := range fake.commands {
		if len(command) > 0 && command[0] == "push" {
			t.Fatalf("comparison materialization attempted a push: %v", command)
		}
	}
}

func TestMaterializeComparisonTargetRejectsRemoteCollision(t *testing.T) {
	target := comparisonTargetProcessTestTarget()
	fake := &comparisonTargetGitFake{remoteURL: "https://github.com/another/widget.git"}
	_, err := materializeComparisonTarget(context.Background(), fake.run, target)
	if err == nil || !strings.Contains(err.Error(), "remote collision") {
		t.Fatalf("collision error = %v, want bounded remote collision", err)
	}
}

func TestWorkspaceTrackerComparisonResolutionNeverFallsBackWhenTargetUnavailable(t *testing.T) {
	tracker := NewWorkspaceTracker(t.TempDir(), newTestLogger(t))
	target := comparisonTargetProcessTestTarget()
	tracker.SetComparisonTarget(&target)
	resolution := tracker.ComparisonResolution()
	if resolution.Ready() {
		t.Fatalf("pending resolution unexpectedly ready: %#v", resolution)
	}
	if !resolution.Explicit || resolution.Status != comparisonTargetStatusUnavailable || resolution.ErrorCode != comparisonTargetErrorPending {
		t.Fatalf("pending resolution = %#v, want explicit unavailable pending", resolution)
	}
	if resolution.Ref != "" {
		t.Fatalf("pending resolution ref = %q, want empty", resolution.Ref)
	}

	tracker.SetComparisonTargetUnavailable(&target, comparisonTargetErrorFetch)
	resolution = tracker.ComparisonResolution()
	if resolution.Ready() {
		t.Fatalf("unavailable resolution unexpectedly ready: %#v", resolution)
	}
	if !resolution.Explicit || resolution.Status != comparisonTargetStatusUnavailable || resolution.ErrorCode != comparisonTargetErrorFetch {
		t.Fatalf("unavailable resolution = %#v, want explicit fetch failure", resolution)
	}
	if resolution.Ref != "" {
		t.Fatalf("unavailable resolution ref = %q, want empty", resolution.Ref)
	}

	tracker.SetComparisonTargetReady(&target, target.ComparisonRef())
	resolution = tracker.ComparisonResolution()
	if !resolution.Explicit || resolution.Status != comparisonTargetStatusReady || resolution.Ref != target.ComparisonRef() {
		t.Fatalf("ready resolution = %#v, want explicit target ref", resolution)
	}
	if !resolution.Ready() {
		t.Fatalf("ready resolution reported unavailable: %#v", resolution)
	}
}

func TestComparisonResolutionReadyRequiresMaterializedReference(t *testing.T) {
	resolution := ComparisonResolution{
		Explicit: true,
		Status:   comparisonTargetStatusReady,
	}
	if resolution.Ready() {
		t.Fatalf("resolution without ref unexpectedly ready: %#v", resolution)
	}
}

func hasComparisonCommand(commands [][]string, want ...string) bool {
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
