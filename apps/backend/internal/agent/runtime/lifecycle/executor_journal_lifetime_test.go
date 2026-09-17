package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestNativeJournalSurvivesAgentctlReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "kandev")
	req := &ExecutorCreateRequest{DurableJournalHostRoot: root, DurableJournalOwnerID: "environment-native"}
	location, err := resolveDurableJournal(req)
	if err != nil {
		t.Fatal(err)
	}
	if capability := journal.CheckStorage(root, req.DurableJournalOwnerID); !capability.Durable {
		t.Fatalf("native retained storage capability = %#v", capability)
	}
	first, err := journal.Open(journal.Config{Path: location.Path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Append(context.Background(), journal.Event{
		SessionID: "session", IncarnationID: "incarnation", StreamID: "stream", Sequence: 1,
		Type: "message", Payload: []byte("before replacement"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := journal.Open(journal.Config{Path: location.Path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	events, _, err := second.Replay(context.Background(), "stream", 0, 10)
	if err != nil || len(events) != 1 || string(events[0].Payload) != "before replacement" {
		t.Fatalf("replayed native journal = %#v, err=%v", events, err)
	}
}

func TestNativeJournalLossAndCleanupSafety(t *testing.T) {
	root := filepath.Join(t.TempDir(), "kandev")
	req := &ExecutorCreateRequest{DurableJournalHostRoot: root, DurableJournalOwnerID: "environment-native"}
	location, err := resolveDurableJournal(req)
	if err != nil {
		t.Fatal(err)
	}
	if capability := journal.CheckStorage(root, req.DurableJournalOwnerID); !capability.Durable {
		t.Fatalf("storage capability = %#v", capability)
	}
	deliveryJournal, err := journal.Open(journal.Config{Path: location.Path})
	if err != nil {
		t.Fatal(err)
	}
	if err := deliveryJournal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := journal.MarkStorageLost(root, req.DurableJournalOwnerID); err != nil {
		t.Fatal(err)
	}
	capability := journal.CheckStorage(root, req.DurableJournalOwnerID)
	if capability.Durable || capability.Reason != "journal_lost" {
		t.Fatalf("lost storage capability = %#v", capability)
	}
}

func TestDockerJournalSurvivesReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docker-root")
	req := &ExecutorCreateRequest{DurableJournalHostRoot: root, DurableJournalOwnerID: "environment-docker"}
	location, err := resolveDurableJournal(req)
	if err != nil {
		t.Fatal(err)
	}
	containerPath, err := durableJournalContainerPath(req)
	if err != nil {
		t.Fatal(err)
	}
	if location.Path == containerPath || filepath.Dir(containerPath) == filepath.Dir(location.Path) {
		t.Fatalf("docker journal was not separated from host path: host=%q container=%q", location.Path, containerPath)
	}
}

func TestDockerJournalLossIsExplicit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "docker-root")
	if capability := journal.CheckStorage(root, "environment-docker"); !capability.Durable {
		t.Fatalf("docker capability = %#v", capability)
	}
	if err := journal.MarkStorageLost(root, "environment-docker"); err != nil {
		t.Fatal(err)
	}
	if capability := journal.CheckStorage(root, "environment-docker"); capability.Reason != "journal_lost" {
		t.Fatalf("docker loss capability = %#v", capability)
	}
}

func TestSSHJournalSurvivesReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ssh-root")
	if capability := journal.CheckStorage(root, "environment-ssh"); !capability.Durable {
		t.Fatalf("ssh capability = %#v", capability)
	}
}

func TestSSHJournalLossIsExplicit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ssh-root")
	if capability := journal.CheckStorage(root, "environment-ssh"); !capability.Durable {
		t.Fatalf("ssh capability = %#v", capability)
	}
	if err := journal.MarkStorageLost(root, "environment-ssh"); err != nil {
		t.Fatal(err)
	}
	if capability := journal.CheckStorage(root, "environment-ssh"); capability.Durable {
		t.Fatalf("ssh loss was advertised as durable: %#v", capability)
	}
}

func TestKubernetesJournalSurvivesReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "kubernetes-root")
	if capability := journal.CheckStorage(root, "environment-kubernetes"); !capability.Durable {
		t.Fatalf("kubernetes capability = %#v", capability)
	}
}

func TestKubernetesJournalLossIsExplicit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "kubernetes-root")
	if capability := journal.CheckStorage(root, "environment-kubernetes"); !capability.Durable {
		t.Fatalf("kubernetes capability = %#v", capability)
	}
	if err := journal.MarkStorageLost(root, "environment-kubernetes"); err != nil {
		t.Fatal(err)
	}
	if capability := journal.CheckStorage(root, "environment-kubernetes"); capability.Durable || capability.Reason != "journal_lost" {
		t.Fatalf("kubernetes loss capability = %#v", capability)
	}
}
