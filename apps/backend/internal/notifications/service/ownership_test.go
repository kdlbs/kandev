package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	userstore "github.com/kandev/kandev/internal/user/store"
	"go.uber.org/zap"
)

// multiUserRepository is a store double that honors provider ownership, so a
// test can observe a leak instead of having the double paper over it.
type multiUserRepository struct {
	mu            sync.Mutex
	providers     []*models.Provider
	subscriptions map[string][]string
	deliveries    []*models.Delivery
}

func newMultiUserRepository() *multiUserRepository {
	return &multiUserRepository{subscriptions: map[string][]string{}}
}

func (r *multiUserRepository) CreateProvider(_ context.Context, provider *models.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	provider.ID = uuid.New().String()
	r.providers = append(r.providers, provider)
	return nil
}

func (r *multiUserRepository) UpdateProvider(_ context.Context, provider *models.Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, existing := range r.providers {
		if existing.ID == provider.ID && existing.UserID == provider.UserID {
			r.providers[index] = provider
			return nil
		}
	}
	return notificationstore.ErrProviderNotFound
}

func (r *multiUserRepository) GetProvider(_ context.Context, userID, id string) (*models.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, provider := range r.providers {
		if provider.ID == id && provider.UserID == userID {
			return provider, nil
		}
	}
	return nil, notificationstore.ErrProviderNotFound
}

func (r *multiUserRepository) ListProvidersByUser(_ context.Context, userID string) ([]*models.Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	owned := make([]*models.Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		if provider.UserID == userID {
			owned = append(owned, provider)
		}
	}
	return owned, nil
}

func (r *multiUserRepository) ListProviderUserIDs(context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	owners := make([]string, 0, len(r.providers))
	for _, provider := range r.providers {
		if !seen[provider.UserID] {
			seen[provider.UserID] = true
			owners = append(owners, provider.UserID)
		}
	}
	return owners, nil
}

func (r *multiUserRepository) DeleteProvider(_ context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, provider := range r.providers {
		if provider.ID == id && provider.UserID == userID {
			r.providers = append(r.providers[:index], r.providers[index+1:]...)
			return nil
		}
	}
	return notificationstore.ErrProviderNotFound
}

func (r *multiUserRepository) ListSubscriptionsByProvider(_ context.Context, providerID string) ([]*models.Subscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	subs := make([]*models.Subscription, 0, len(r.subscriptions[providerID]))
	for _, eventType := range r.subscriptions[providerID] {
		subs = append(subs, &models.Subscription{ProviderID: providerID, EventType: eventType, Enabled: true})
	}
	return subs, nil
}

func (r *multiUserRepository) ReplaceSubscriptions(_ context.Context, providerID, _ string, events []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscriptions[providerID] = append([]string(nil), events...)
	return nil
}

func (r *multiUserRepository) InsertDelivery(_ context.Context, delivery *models.Delivery) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.deliveries {
		if existing.ProviderID == delivery.ProviderID && existing.EventType == delivery.EventType && existing.OccurrenceID == delivery.OccurrenceID {
			return false, nil
		}
	}
	r.deliveries = append(r.deliveries, delivery)
	return true, nil
}

func (r *multiUserRepository) DeleteDelivery(context.Context, string, string, string) error {
	return nil
}

func (r *multiUserRepository) Close() error { return nil }

// seedProvider stores an enabled provider whose config carries a webhook URL
// unique to its owner, so a captured message names the account it leaked to.
func (r *multiUserRepository) seedProvider(userID, webhook string, events ...string) *models.Provider {
	provider := &models.Provider{
		UserID:  userID,
		Name:    webhook,
		Type:    models.ProviderTypeLocal,
		Config:  map[string]interface{}{"urls": webhook},
		Enabled: true,
	}
	_ = r.CreateProvider(context.Background(), provider)
	_ = r.ReplaceSubscriptions(context.Background(), provider.ID, userID, events)
	return provider
}

// ownedWorkspaceTasks resolves every task into one workspace with one owner.
type ownedWorkspaceTasks struct {
	workspaceID string
	ownerID     string
}

func (t ownedWorkspaceTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	return &taskmodels.Task{ID: id, WorkspaceID: t.workspaceID}, nil
}

func (t ownedWorkspaceTasks) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if id != t.workspaceID {
		return nil, errors.New("workspace not found")
	}
	return &taskmodels.Workspace{ID: id, OwnerID: t.ownerID}, nil
}

func newOwnershipTestService(t *testing.T, repo notificationstore.Repository, tasks TaskContextReader, authEnforced bool) (*Service, *captureProvider) {
	t.Helper()
	// Keep the auto-provisioned system provider out so assertions count only
	// the providers the test seeded; SystemProvider.Available() is true on
	// desktop platforms and would add an extra delivery.
	t.Setenv(desktopNativeNotificationsEnv, "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	svc := NewService(repo, tasks, nil, log, func() bool { return authEnforced })
	capture := &captureProvider{}
	svc.providers[models.ProviderTypeLocal] = capture
	return svc, capture
}

// webhooksOf lists the provider config each captured message was sent with,
// which is the value that would reach an external service.
func webhooksOf(messages []providers.Message) []string {
	urls := make([]string, 0, len(messages))
	for _, message := range messages {
		url, _ := message.Config["urls"].(string)
		urls = append(urls, url)
	}
	return urls
}

func TestTaskNotificationReachesOnlyTheOwningWorkspacesUser(t *testing.T) {
	repo := newMultiUserRepository()
	repo.seedProvider("user-a", "slack://user-a", EventTaskSessionTurnFinished)
	repo.seedProvider("user-b", "slack://user-b", EventTaskSessionTurnFinished)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-a", ownerID: "user-a"}, true)

	svc.HandleTaskTurnFinished(context.Background(), "task-in-workspace-a", "session-1", "turn-1")

	got := webhooksOf(capture.messages)
	if len(got) != 1 || got[0] != "slack://user-a" {
		t.Fatalf("delivered to %#v, want only slack://user-a", got)
	}
	if len(repo.deliveries) != 1 || repo.deliveries[0].UserID != "user-a" {
		t.Fatalf("delivery rows = %#v, want a single row owned by user-a", repo.deliveries)
	}
	for _, message := range capture.messages {
		if message.UserID != "user-a" {
			t.Fatalf("message routed to %q, want user-a", message.UserID)
		}
	}
}

func TestClarificationNotificationDoesNotReachAnotherUsersWebhook(t *testing.T) {
	repo := newMultiUserRepository()
	repo.seedProvider("user-a", "slack://user-a", EventTaskSessionClarificationAsked)
	repo.seedProvider("user-b", "slack://user-b", EventTaskSessionClarificationAsked)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-b", ownerID: "user-b"}, true)

	svc.HandleClarificationRequested(context.Background(), "task-in-workspace-b", "session-9", "pending-1")

	got := webhooksOf(capture.messages)
	if len(got) != 1 || got[0] != "slack://user-b" {
		t.Fatalf("delivered to %#v, want only slack://user-b", got)
	}
}

func TestOfficeInboxItemFollowsItsWorkspaceOwner(t *testing.T) {
	repo := newMultiUserRepository()
	repo.seedProvider("user-a", "slack://user-a", EventOfficeInboxItem)
	repo.seedProvider("user-b", "slack://user-b", EventOfficeInboxItem)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-b", ownerID: "user-b"}, true)

	svc.HandleInboxItem(context.Background(), "workspace-b", "approval", "Deploy to production")

	got := webhooksOf(capture.messages)
	if len(got) != 1 || got[0] != "slack://user-b" {
		t.Fatalf("inbox item delivered to %#v, want only slack://user-b", got)
	}
}

func TestSecondUserGetsTheirOwnSeededDefaultProviders(t *testing.T) {
	repo := newMultiUserRepository()
	svc, _ := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-b", ownerID: "user-b"}, true)

	// user-b has never opened notification settings; a task event in their
	// workspace must still provision their own defaults rather than reuse
	// somebody else's.
	svc.HandleClarificationRequested(context.Background(), "task-in-workspace-b", "session-1", "pending-1")

	owned, err := repo.ListProvidersByUser(context.Background(), "user-b")
	if err != nil {
		t.Fatalf("list user-b providers: %v", err)
	}
	if len(owned) != 1 || owned[0].Type != models.ProviderTypeLocal {
		t.Fatalf("user-b providers = %#v, want one seeded local provider", owned)
	}
	if defaultOwned, _ := repo.ListProvidersByUser(context.Background(), userstore.DefaultUserID); len(defaultOwned) != 0 {
		t.Fatalf("default user gained providers = %#v, want none", defaultOwned)
	}
}

func TestInstanceWideUpdateNoticeReachesEveryProviderOwner(t *testing.T) {
	repo := newMultiUserRepository()
	repo.seedProvider("user-a", "slack://user-a", EventSystemUpdateAvailable)
	repo.seedProvider("user-b", "slack://user-b", EventSystemUpdateAvailable)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-a", ownerID: "user-a"}, true)

	svc.HandleUpdateAvailable(context.Background(), "v9.9.9", "https://example.test/releases/v9.9.9")

	got := webhooksOf(capture.messages)
	if len(got) != 2 || got[0] != "slack://user-a" || got[1] != "slack://user-b" {
		t.Fatalf("update notice delivered to %#v, want both owners", got)
	}
}

func TestSingleUserInstallKeepsDeliveringToTheDefaultUser(t *testing.T) {
	repo := newMultiUserRepository()
	// Authentication disabled: workspaces are still unowned (owner_id='').
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-1", ownerID: ""}, false)

	// The seeded default provider subscribes to clarifications, not turns.
	svc.HandleClarificationRequested(context.Background(), "task-1", "session-1", "pending-1")

	owned, err := repo.ListProvidersByUser(context.Background(), userstore.DefaultUserID)
	if err != nil {
		t.Fatalf("list default providers: %v", err)
	}
	if len(owned) != 1 {
		t.Fatalf("default user providers = %#v, want the single seeded default", owned)
	}
	if len(capture.messages) != 1 || capture.messages[0].UserID != userstore.DefaultUserID {
		t.Fatalf("messages = %#v, want one addressed to the default user", capture.messages)
	}
	if len(repo.deliveries) != 1 || repo.deliveries[0].UserID != userstore.DefaultUserID {
		t.Fatalf("delivery rows = %#v, want one owned by the default user", repo.deliveries)
	}
}

func TestServiceRefusesToTouchAnotherUsersProvider(t *testing.T) {
	repo := newMultiUserRepository()
	owned := repo.seedProvider("user-a", "slack://user-a", EventTaskSessionTurnFinished)
	svc, capture := newOwnershipTestService(t, repo, ownedWorkspaceTasks{workspaceID: "workspace-a", ownerID: "user-a"}, true)

	stolen := "stolen"
	if _, err := svc.UpdateProvider(context.Background(), "user-b", owned.ID, ProviderUpdate{Name: &stolen}); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("foreign update error = %v, want ErrProviderNotFound", err)
	}
	if err := svc.TestProvider(context.Background(), "user-b", owned.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("foreign test error = %v, want ErrProviderNotFound", err)
	}
	if err := svc.DeleteProvider(context.Background(), "user-b", owned.ID); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("foreign delete error = %v, want ErrProviderNotFound", err)
	}

	missingErr := svc.TestProvider(context.Background(), "user-b", "no-such-provider")
	foreignErr := svc.TestProvider(context.Background(), "user-b", owned.ID)
	if missingErr.Error() != foreignErr.Error() {
		t.Fatalf("foreign error %q differs from missing error %q", foreignErr, missingErr)
	}
	if len(capture.messages) != 0 {
		t.Fatalf("foreign test fired %#v, want no notification", capture.messages)
	}
	after, err := repo.GetProvider(context.Background(), "user-a", owned.ID)
	if err != nil || after.Name != "slack://user-a" {
		t.Fatalf("provider mutated by a foreign caller: %#v (%v)", after, err)
	}
}
