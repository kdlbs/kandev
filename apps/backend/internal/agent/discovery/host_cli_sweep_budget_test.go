package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostcli"
)

type budgetBlockedAgent struct {
	discoveryTestAgent
	release <-chan struct{}
}

func (a *budgetBlockedAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	select {
	case <-a.release:
		return a.discoveryTestAgent.IsInstalled(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type budgetVersionRunner struct {
	versionRunner
	started chan struct{}
}

func (r *budgetVersionRunner) Output(ctx context.Context, argv []string) (string, error) {
	close(r.started)
	return r.versionRunner.Output(ctx, argv)
}

func TestDetectAllChecksHostCLIWhileAnotherAgentIsInstalling(t *testing.T) {
	release := make(chan struct{})
	slow := &budgetBlockedAgent{
		discoveryTestAgent: discoveryTestAgent{id: "slow", discovery: &agents.DiscoveryResult{Available: true}},
		release:            release,
	}
	runner := &budgetVersionRunner{versionRunner: versionRunner{output: "testcli 1.2.3"}, started: make(chan struct{})}
	r := newHostCLIRegistry(t, []agents.Agent{slow, cliTestAgent("fast", "/testcli")}, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan []Availability, 1)
	go func() { done <- r.detectAll(ctx) }()
	versionStarted := false
	select {
	case <-runner.started:
		versionStarted = true
	case <-time.After(2 * time.Second):
	}
	close(release)
	results := <-done
	if !versionStarted {
		t.Error("a healthy CLI version check waited for another agent's installation check")
	}
	fast, ok := findAvailability(results, "fast")
	if !ok || fast.CLIVersion != "1.2.3" {
		t.Fatalf("fast availability = %#v, found=%v", fast, ok)
	}
}

var _ hostcli.Runner = (*budgetVersionRunner)(nil)

type cancelledBudgetRunner struct {
	versionRunner
	started chan struct{}
}

func (r *cancelledBudgetRunner) Output(ctx context.Context, _ []string) (string, error) {
	close(r.started)
	<-ctx.Done()
	return "", ctx.Err()
}

func TestDetectAllRetainsAvailabilityWhenHostCLIIsCancelled(t *testing.T) {
	runner := &cancelledBudgetRunner{started: make(chan struct{})}
	r := newHostCLIRegistry(t, []agents.Agent{cliTestAgent("fast", "/testcli")}, runner)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []Availability, 1)
	go func() { done <- r.detectAll(ctx) }()
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("version probe did not start")
	}
	cancel()
	fast, ok := findAvailability(<-done, "fast")
	if !ok || !fast.Available {
		t.Fatalf("completed installation result was lost: %#v, found=%v", fast, ok)
	}
}
