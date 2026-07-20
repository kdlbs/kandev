package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxGitHubWebhookPayloadSize = 10 * 1024 * 1024
	webhookDeliveryStaleAfter   = 5 * time.Minute
)

var ErrInvalidWebhookSignature = errors.New("invalid GitHub webhook signature")

type githubWebhookStore interface {
	ClaimWebhookDelivery(context.Context, *WebhookDelivery, time.Time) (WebhookDeliveryClaim, error)
	CompleteWebhookDelivery(context.Context, string, WebhookDeliveryStatus, string, time.Time) error
	ListWorkspaceConnectionsByInstallation(context.Context, int64) ([]*WorkspaceConnection, error)
	TransitionWorkspaceInstallationConnection(
		context.Context,
		*WorkspaceConnection,
		*WorkspaceConnection,
	) (bool, error)
	ListUserConnectionsByGitHubUser(context.Context, int64) ([]*UserConnection, error)
}

type personalConnectionRevoker interface {
	RevokePersonalConnection(context.Context, string, string) error
}

type personalAuthorizationReconciler interface {
	ReconcileAuthorizationRevocation(context.Context, *UserConnection) (bool, error)
}

type GitHubWebhookReconciliation struct {
	Installations appInstallationVerifier
	Personal      personalAuthorizationReconciler
}

type InstallationRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type InstallationRepositoriesChange struct {
	WorkspaceID          string
	InstallationID       int64
	ConnectionSource     ConnectionSource
	CredentialGeneration int64
	Action               string
	Added                []InstallationRepository
	Removed              []InstallationRepository
}

type installationRepositoryUpdater interface {
	ApplyInstallationRepositories(context.Context, InstallationRepositoriesChange) (bool, error)
}

type GitHubWebhookRequest struct {
	DeliveryID string
	Event      string
	Signature  string
	Payload    []byte
}

type GitHubWebhookResult struct {
	Duplicate bool
	Status    WebhookDeliveryStatus
	Affected  int
}

type installationWebhookEvent struct {
	Action       string `json:"action"`
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
	} `json:"installation"`
}

type installationTransition struct {
	status      ConnectionStatus
	lastError   string
	login       string
	accountType string
	verified    bool
}

type GitHubWebhookService struct {
	secret             []byte
	store              githubWebhookStore
	repos              installationRepositoryUpdater
	personal           personalConnectionRevoker
	installations      appInstallationVerifier
	personalReconciler personalAuthorizationReconciler
	now                func() time.Time
}

func NewGitHubWebhookService(
	secret string,
	store githubWebhookStore,
	repositories installationRepositoryUpdater,
	personal personalConnectionRevoker,
	reconciliation ...GitHubWebhookReconciliation,
) *GitHubWebhookService {
	service := &GitHubWebhookService{
		secret: []byte(secret), store: store, repos: repositories, personal: personal, now: time.Now,
	}
	if len(reconciliation) > 0 {
		service.installations = reconciliation[0].Installations
		service.personalReconciler = reconciliation[0].Personal
	}
	return service
}

func (s *GitHubWebhookService) Authenticates(request GitHubWebhookRequest) bool {
	return s != nil && len(s.secret) > 0 && request.DeliveryID != "" && request.Event != "" &&
		len(request.Payload) <= maxGitHubWebhookPayloadSize &&
		validGitHubWebhookSignature(s.secret, request.Payload, request.Signature)
}

func (s *GitHubWebhookService) Handle(
	ctx context.Context,
	request GitHubWebhookRequest,
) (GitHubWebhookResult, error) {
	if s == nil || s.store == nil || len(s.secret) == 0 {
		return GitHubWebhookResult{}, errors.New("GitHub webhook service is not configured")
	}
	if request.DeliveryID == "" || request.Event == "" || len(request.Payload) > maxGitHubWebhookPayloadSize {
		return GitHubWebhookResult{}, errors.New("invalid GitHub webhook request")
	}
	if !validGitHubWebhookSignature(s.secret, request.Payload, request.Signature) {
		return GitHubWebhookResult{}, ErrInvalidWebhookSignature
	}
	now := s.now().UTC()
	claim, err := s.store.ClaimWebhookDelivery(ctx, &WebhookDelivery{
		DeliveryID: request.DeliveryID,
		Event:      request.Event,
		Status:     WebhookDeliveryStatusReceived,
		ReceivedAt: now,
	}, now.Add(-webhookDeliveryStaleAfter))
	if err != nil {
		return GitHubWebhookResult{}, fmt.Errorf("record GitHub webhook delivery: %w", err)
	}
	if !claim.Acquired {
		return GitHubWebhookResult{Duplicate: true, Status: claim.Status}, nil
	}

	affected, result, processErr := s.process(ctx, request.Event, request.Payload)
	status := WebhookDeliveryStatusProcessed
	if affected == 0 {
		status = WebhookDeliveryStatusIgnored
	}
	if processErr != nil {
		status = WebhookDeliveryStatusFailed
		result = processErr.Error()
	}
	completeErr := s.store.CompleteWebhookDelivery(ctx, request.DeliveryID, status, result, s.now().UTC())
	if processErr != nil || completeErr != nil {
		return GitHubWebhookResult{Status: status, Affected: affected}, errors.Join(processErr, completeErr)
	}
	return GitHubWebhookResult{Status: status, Affected: affected}, nil
}

func (s *GitHubWebhookService) process(ctx context.Context, event string, payload []byte) (int, string, error) {
	switch event {
	case "installation":
		return s.processInstallation(ctx, payload)
	case "installation_repositories":
		return s.processInstallationRepositories(ctx, payload)
	case "github_app_authorization":
		return s.processAuthorization(ctx, payload)
	default:
		return 0, "event ignored", nil
	}
}

func (s *GitHubWebhookService) processInstallation(
	ctx context.Context,
	payload []byte,
) (int, string, error) {
	var event installationWebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return 0, "", fmt.Errorf("decode installation webhook: %w", err)
	}
	transition, supported, err := s.installationTransition(ctx, event)
	if err != nil {
		return 0, "", err
	}
	if !supported || event.Installation.ID <= 0 {
		return 0, "installation action ignored", nil
	}
	connections, err := s.store.ListWorkspaceConnectionsByInstallation(ctx, event.Installation.ID)
	if err != nil {
		return 0, "", fmt.Errorf("load installation bindings: %w", err)
	}
	affected := 0
	for _, connection := range connections {
		if !shouldApplyInstallationTransition(connection, event, transition) {
			continue
		}
		updated, err := s.applyInstallationTransition(ctx, connection, transition)
		if err != nil {
			return affected, "", fmt.Errorf("update installation binding: %w", err)
		}
		if !updated {
			continue
		}
		affected++
	}
	return affected, fmt.Sprintf("installation %s: %d binding(s)", event.Action, affected), nil
}

func (s *GitHubWebhookService) installationTransition(
	ctx context.Context,
	event installationWebhookEvent,
) (installationTransition, bool, error) {
	status, lastError, supported := installationWebhookStatus(event.Action)
	transition := installationTransition{
		status: status, lastError: lastError,
		login: event.Installation.Account.Login, accountType: event.Installation.Account.Type,
	}
	if !supported || event.Installation.ID <= 0 || s.installations == nil {
		return transition, supported, nil
	}
	status, lastError, login, accountType, err := s.reconcileInstallation(ctx, event.Installation.ID)
	if err != nil {
		return installationTransition{}, false, err
	}
	return installationTransition{
		status: status, lastError: lastError, login: login, accountType: accountType, verified: true,
	}, true, nil
}

func shouldApplyInstallationTransition(
	connection *WorkspaceConnection,
	event installationWebhookEvent,
	transition installationTransition,
) bool {
	if !matchesInstallation(connection, event.Installation.ID) {
		return false
	}
	if !transition.verified {
		return canApplyInstallationTransition(connection.Status, event.Action)
	}
	return connection.Status != transition.status ||
		(transition.login != "" && connection.InstallationAccountLogin != transition.login) ||
		(transition.accountType != "" && connection.InstallationAccountType != transition.accountType)
}

func (s *GitHubWebhookService) applyInstallationTransition(
	ctx context.Context,
	connection *WorkspaceConnection,
	transition installationTransition,
) (bool, error) {
	expected := *connection
	next := expected
	next.Status = transition.status
	next.LastError = transition.lastError
	next.CredentialGeneration++
	if transition.login != "" {
		next.InstallationAccountLogin = transition.login
	}
	if transition.accountType != "" {
		next.InstallationAccountType = transition.accountType
	}
	updated, err := s.store.TransitionWorkspaceInstallationConnection(ctx, &expected, &next)
	if updated {
		*connection = next
	}
	return updated, err
}

func (s *GitHubWebhookService) processInstallationRepositories(
	ctx context.Context,
	payload []byte,
) (int, string, error) {
	var event struct {
		Action       string `json:"action"`
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
		Added   []InstallationRepository `json:"repositories_added"`
		Removed []InstallationRepository `json:"repositories_removed"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return 0, "", fmt.Errorf("decode installation repositories webhook: %w", err)
	}
	if event.Installation.ID <= 0 || (event.Action != "added" && event.Action != "removed") {
		return 0, "installation repositories action ignored", nil
	}
	connections, err := s.store.ListWorkspaceConnectionsByInstallation(ctx, event.Installation.ID)
	if err != nil {
		return 0, "", fmt.Errorf("load installation bindings: %w", err)
	}
	affected := 0
	for _, connection := range connections {
		if !matchesInstallation(connection, event.Installation.ID) {
			continue
		}
		if s.repos == nil {
			return affected, "", errors.New("installation repository updater is not configured")
		}
		updated, err := s.repos.ApplyInstallationRepositories(ctx, InstallationRepositoriesChange{
			WorkspaceID:          connection.WorkspaceID,
			InstallationID:       event.Installation.ID,
			ConnectionSource:     connection.Source,
			CredentialGeneration: connection.CredentialGeneration,
			Action:               event.Action,
			Added:                event.Added,
			Removed:              event.Removed,
		})
		if err != nil {
			return affected, "", fmt.Errorf("update installation repository access: %w", err)
		}
		if updated {
			affected++
		}
	}
	return affected, fmt.Sprintf("installation repositories %s: %d binding(s)", event.Action, affected), nil
}

func (s *GitHubWebhookService) processAuthorization(
	ctx context.Context,
	payload []byte,
) (int, string, error) {
	var event struct {
		Action string `json:"action"`
		Sender struct {
			ID int64 `json:"id"`
		} `json:"sender"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return 0, "", fmt.Errorf("decode GitHub App authorization webhook: %w", err)
	}
	if event.Action != "revoked" || event.Sender.ID <= 0 {
		return 0, "authorization action ignored", nil
	}
	connections, err := s.store.ListUserConnectionsByGitHubUser(ctx, event.Sender.ID)
	if err != nil {
		return 0, "", fmt.Errorf("load personal GitHub bindings: %w", err)
	}
	affected := 0
	for _, connection := range connections {
		if connection == nil || connection.GitHubUserID != event.Sender.ID {
			continue
		}
		if s.personalReconciler != nil {
			revoked, reconcileErr := s.personalReconciler.ReconcileAuthorizationRevocation(ctx, connection)
			if reconcileErr != nil {
				return affected, "", fmt.Errorf("reconcile personal GitHub connection: %w", reconcileErr)
			}
			if revoked {
				affected++
			}
			continue
		}
		if s.personal == nil {
			return affected, "", errors.New("personal GitHub revoker is not configured")
		}
		if err := s.personal.RevokePersonalConnection(ctx, connection.WorkspaceID, connection.UserID); err != nil {
			return affected, "", fmt.Errorf("revoke personal GitHub connection: %w", err)
		}
		affected++
	}
	return affected, fmt.Sprintf("authorization revoked: %d binding(s)", affected), nil
}

func (s *GitHubWebhookService) reconcileInstallation(
	ctx context.Context,
	installationID int64,
) (ConnectionStatus, string, string, string, error) {
	installation, err := s.installations.GetInstallation(ctx, installationID)
	if err != nil {
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			return ConnectionStatusRevoked, "GitHub App installation deleted", "", "", nil
		}
		return "", "", "", "", fmt.Errorf("reconcile GitHub App installation: %w", err)
	}
	if installation.ID != installationID {
		return "", "", "", "", errors.New("reconciled GitHub App installation ID does not match webhook")
	}
	if installation.SuspendedAt != nil {
		return ConnectionStatusSuspended, "GitHub App installation suspended",
			installation.AccountLogin, installation.AccountType, nil
	}
	return ConnectionStatusActive, "", installation.AccountLogin, installation.AccountType, nil
}

func validGitHubWebhookSignature(secret, payload []byte, provided string) bool {
	if !strings.HasPrefix(provided, "sha256=") {
		return false
	}
	providedMAC, err := hex.DecodeString(strings.TrimPrefix(provided, "sha256="))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return hmac.Equal(mac.Sum(nil), providedMAC)
}

func installationWebhookStatus(action string) (ConnectionStatus, string, bool) {
	switch action {
	case "suspend":
		return ConnectionStatusSuspended, "GitHub App installation suspended", true
	case "unsuspend":
		return ConnectionStatusActive, "", true
	case "deleted":
		return ConnectionStatusRevoked, "GitHub App installation deleted", true
	default:
		return "", "", false
	}
}

func canApplyInstallationTransition(current ConnectionStatus, action string) bool {
	switch action {
	case "suspend":
		return current != ConnectionStatusSuspended && current != ConnectionStatusRevoked
	case "unsuspend":
		return current == ConnectionStatusSuspended || current == ConnectionStatusInvalid
	case "deleted":
		return current != ConnectionStatusRevoked
	default:
		return false
	}
}

func matchesInstallation(connection *WorkspaceConnection, installationID int64) bool {
	return connection != nil && connection.Source == ConnectionSourceGitHubAppInstallation &&
		connection.InstallationID != nil && *connection.InstallationID == installationID
}
