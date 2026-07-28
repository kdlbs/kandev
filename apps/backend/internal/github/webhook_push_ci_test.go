package github

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func newTestMemoryBus(t *testing.T) *bus.MemoryEventBus {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return bus.NewMemoryEventBus(log)
}

func TestGitHubWebhookPushPublishesPushReceived(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {{
			WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: ConnectionStatusActive,
		}},
	}}
	eventBus := newTestMemoryBus(t)
	var received *GitHubPushEventPayload
	if _, err := eventBus.Subscribe(events.GitHubPushReceived, func(_ context.Context, e *bus.Event) error {
		received = e.Data.(*GitHubPushEventPayload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, eventBus)
	payload := []byte(`{
		"ref":"refs/heads/main","after":"abc1234",
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42},"pusher":{"name":"alice"},
		"head_commit":{"message":"feat: add widget"}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-push-1", "push", payload)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 1 || result.Status != WebhookDeliveryStatusProcessed {
		t.Fatalf("result = %+v", result)
	}
	if received == nil {
		t.Fatal("expected push event to be published")
	}
	if received.WorkspaceIDs[0] != "workspace-1" || received.Owner != "acme" || received.Name != "repo" ||
		received.Branch != "main" || received.SHA != "abc1234" || received.PusherLogin != "alice" ||
		received.HeadCommitMsg != "feat: add widget" {
		t.Fatalf("received = %+v", received)
	}
}

func TestGitHubWebhookResolvesDistinctActiveWorkspaces(t *testing.T) {
	installationID := int64(42)
	conn := func(ws string, status ConnectionStatus) *WorkspaceConnection {
		return &WorkspaceConnection{
			WorkspaceID: ws, AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: status,
		}
	}
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {
			conn("workspace-1", ConnectionStatusActive),
			conn("workspace-1", ConnectionStatusActive),    // duplicate → collapsed
			conn("workspace-2", ConnectionStatusActive),    // distinct → kept
			conn("workspace-3", ConnectionStatusSuspended), // suspended → excluded
			conn("workspace-4", ConnectionStatusRevoked),   // revoked → excluded
		},
	}}
	eventBus := newTestMemoryBus(t)
	var received *GitHubPushEventPayload
	if _, err := eventBus.Subscribe(events.GitHubPushReceived, func(_ context.Context, e *bus.Event) error {
		received = e.Data.(*GitHubPushEventPayload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, eventBus)
	payload := []byte(`{
		"ref":"refs/heads/main","after":"abc1234",
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42},"pusher":{"name":"alice"}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-multi-ws", "push", payload)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 2 {
		t.Fatalf("expected 2 distinct active workspaces, got affected=%d", result.Affected)
	}
	if received == nil {
		t.Fatal("expected push event to be published")
	}
	got := map[string]bool{}
	for _, ws := range received.WorkspaceIDs {
		got[ws] = true
	}
	if len(received.WorkspaceIDs) != 2 || !got["workspace-1"] || !got["workspace-2"] ||
		got["workspace-3"] || got["workspace-4"] {
		t.Fatalf("expected [workspace-1 workspace-2], got %v", received.WorkspaceIDs)
	}
}

func TestGitHubWebhookPushIgnoresBranchDelete(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {{
			WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: ConnectionStatusActive,
		}},
	}}
	eventBus := newTestMemoryBus(t)
	published := false
	if _, err := eventBus.Subscribe(events.GitHubPushReceived, func(_ context.Context, _ *bus.Event) error {
		published = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, eventBus)
	payload := []byte(`{
		"ref":"refs/heads/main","after":"0000000000000000000000000000000000000000","deleted":true,
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42},"pusher":{"name":"alice"}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-push-2", "push", payload)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 0 || result.Status != WebhookDeliveryStatusIgnored {
		t.Fatalf("result = %+v", result)
	}
	if published {
		t.Fatal("expected branch-delete push not to publish an event")
	}
}

func TestGitHubWebhookPushIgnoresNonBranchRef(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {{
			WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: ConnectionStatusActive,
		}},
	}}
	eventBus := newTestMemoryBus(t)
	published := false
	if _, err := eventBus.Subscribe(events.GitHubPushReceived, func(_ context.Context, _ *bus.Event) error {
		published = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, eventBus)
	payload := []byte(`{
		"ref":"refs/tags/v1.0.0","after":"abc1234",
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42},"pusher":{"name":"alice"}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-push-3", "push", payload)

	if _, err := service.Handle(context.Background(), request); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if published {
		t.Fatal("expected tag push not to publish an event")
	}
}

func TestGitHubWebhookCheckRunPublishesCompletedEvent(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {{
			WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: ConnectionStatusActive,
		}},
	}}
	eventBus := newTestMemoryBus(t)
	var received *GitHubCheckRunEventPayload
	if _, err := eventBus.Subscribe(events.GitHubCheckRunCompleted, func(_ context.Context, e *bus.Event) error {
		received = e.Data.(*GitHubCheckRunEventPayload)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, eventBus)
	payload := []byte(`{
		"action":"completed",
		"check_run":{
			"id":555,"name":"build","head_sha":"abc1234","conclusion":"failure",
			"html_url":"https://github.com/acme/repo/runs/555",
			"check_suite":{"head_branch":"main"}
		},
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-ci-1", "check_run", payload)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 1 || result.Status != WebhookDeliveryStatusProcessed {
		t.Fatalf("result = %+v", result)
	}
	if received == nil {
		t.Fatal("expected check_run event to be published")
	}
	if received.WorkspaceIDs[0] != "workspace-1" || received.Owner != "acme" || received.Name != "repo" ||
		received.Branch != "main" || received.SHA != "abc1234" || received.CheckName != "build" ||
		received.Conclusion != "failure" || received.CheckRunID != 555 ||
		received.HTMLURL != "https://github.com/acme/repo/runs/555" {
		t.Fatalf("received = %+v", received)
	}
}

func TestGitHubWebhookCheckRunIgnoresNonCompletedAction(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {{
			WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: ConnectionStatusActive,
		}},
	}}
	eventBus := newTestMemoryBus(t)
	published := false
	if _, err := eventBus.Subscribe(events.GitHubCheckRunCompleted, func(_ context.Context, _ *bus.Event) error {
		published = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, eventBus)
	payload := []byte(`{
		"action":"in_progress",
		"check_run":{
			"id":555,"name":"build","head_sha":"abc1234","conclusion":"",
			"check_suite":{"head_branch":"main"}
		},
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-ci-2", "check_run", payload)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 0 || result.Status != WebhookDeliveryStatusIgnored {
		t.Fatalf("result = %+v", result)
	}
	if published {
		t.Fatal("expected non-completed check_run not to publish an event")
	}
}

func TestGitHubWebhookPublishNilPublisherIsNoOp(t *testing.T) {
	installationID := int64(42)
	store := &githubWebhookMemoryStore{workspaces: map[int64][]*WorkspaceConnection{
		installationID: {{
			WorkspaceID: "workspace-1", AppRegistrationID: "registration-work",
			Source: ConnectionSourceGitHubAppInstallation, InstallationID: &installationID,
			Status: ConnectionStatusActive,
		}},
	}}
	service := NewAppRegistrationWebhookService("registration-work", "work-secret", store, nil, nil, nil)
	payload := []byte(`{
		"ref":"refs/heads/main","after":"abc1234",
		"repository":{"name":"repo","owner":{"login":"acme"}},
		"installation":{"id":42},"pusher":{"name":"alice"}
	}`)
	request := signedWebhookRequest("work-secret", "delivery-push-nil", "push", payload)

	result, err := service.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Affected != 1 || result.Status != WebhookDeliveryStatusProcessed {
		t.Fatalf("result = %+v", result)
	}
}
