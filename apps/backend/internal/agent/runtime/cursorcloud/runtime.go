// Package cursorcloud implements the managed Cursor Cloud runtime over the
// durable binding and operation journal.
package cursorcloud

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	LaunchMetadataKey  = "cursor_cloud_launch"
	initialTurnPrefix  = "cursor-cloud-initial:"
	leaseDuration      = 2 * time.Minute
	runStatusRunning   = "RUNNING"
	runStatusError     = "ERROR"
	runStatusCancelled = "CANCELLED"
	runStatusExpired   = "EXPIRED"
)

var ErrCancellationPending = errors.New("cursor cloud cancellation is pending remote confirmation")

type RemoteLivenessState string

const (
	RemoteLivenessLive     RemoteLivenessState = "live"
	RemoteLivenessTerminal RemoteLivenessState = "terminal"
	RemoteLivenessAbsent   RemoteLivenessState = "absent"
	RemoteLivenessUnknown  RemoteLivenessState = "unknown"
)

type Repository interface {
	ReserveManagedAgentStart(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation, string, time.Time) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error)
	ReserveManagedAgentOperation(context.Context, *models.ManagedAgentOperation, int64, string, time.Time) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error)
	ClaimManagedAgentDispatchLease(context.Context, string, string, int64, string, time.Time) (*models.ManagedAgentBinding, error)
	GetManagedAgentBindingBySession(context.Context, string) (*models.ManagedAgentBinding, error)
	GetManagedAgentBindingByExecution(context.Context, string) (*models.ManagedAgentBinding, error)
	GetManagedAgentOperationByPromptTurnID(context.Context, string) (*models.ManagedAgentOperation, error)
	GetManagedAgentOperation(context.Context, string) (*models.ManagedAgentOperation, error)
	GetManagedAgentLatestOperation(context.Context, string) (*models.ManagedAgentOperation, error)
	ListActiveManagedAgentBindings(context.Context) ([]*models.ManagedAgentBinding, error)
	CompareAndSwapManagedAgentOperation(context.Context, models.ManagedAgentOperationUpdate) (*models.ManagedAgentOperation, error)
	AcknowledgeManagedAgentCompletion(context.Context, string) error
	GetManagedAgentStreamCheckpoint(context.Context, string, string) (*models.ManagedAgentStreamCheckpoint, error)
	CommitManagedAgentStreamEvent(context.Context, models.ManagedAgentStreamEvent) (bool, error)
}

type Provider interface {
	CreateAgent(context.Context, provider.CreateAgentRequest) (provider.CreateAgentResponse, error)
	CreateRun(context.Context, string, provider.CreateRunRequest) (provider.Run, error)
	GetAgent(context.Context, string) (provider.Agent, error)
	GetRun(context.Context, string, string) (provider.Run, error)
	CancelRun(context.Context, string, string) error
	StreamRun(context.Context, string, string, string, func(provider.StreamEvent) error) (provider.StreamResult, error)
}

type ClientFactory func(context.Context, *models.ManagedAgentBinding) (Provider, error)

type MCPGrant struct {
	URL   string
	Token string
}

type GrantIssuer interface {
	Issue(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation) (MCPGrant, error)
}

type GrantIssuerFunc func(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation) (MCPGrant, error)

func (f GrantIssuerFunc) Issue(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) (MCPGrant, error) {
	return f(ctx, binding, operation)
}

type LaunchInput struct {
	Binding   *models.ManagedAgentBinding
	Operation *models.ManagedAgentOperation
}

type StreamProjection struct {
	Message        *models.Message
	Payload        *lifecycle.AgentStreamEventPayload
	TerminalStatus string
	AppendMessage  bool
}

type StreamProjector func(context.Context, *models.ManagedAgentBinding, *models.ManagedAgentOperation, provider.StreamEvent) (*StreamProjection, error)
type StreamPublisher func(context.Context, *lifecycle.AgentStreamEventPayload)

type Config struct {
	Repository    Repository
	ClientFactory ClientFactory
	GrantIssuer   GrantIssuer
	Enabled       func() bool
	Now           func() time.Time
	NewID         func() string
	ProjectStream StreamProjector
	PublishStream StreamPublisher
}

type Runtime struct {
	repository    Repository
	clientFactory ClientFactory
	grantIssuer   GrantIssuer
	enabled       func() bool
	now           func() time.Time
	newID         func() string
	projectStream StreamProjector
	publishStream StreamPublisher
	fallbackMu    sync.Mutex
	fallbackAt    map[string]time.Time
}

var _ runtime.Runtime = (*Runtime)(nil)

func New(config Config) (*Runtime, error) {
	if config.Repository == nil || config.ClientFactory == nil || config.GrantIssuer == nil {
		return nil, errors.New("cursor cloud runtime dependencies are incomplete")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.NewID == nil {
		config.NewID = uuid.NewString
	}
	if config.Enabled == nil {
		config.Enabled = func() bool { return true }
	}
	return &Runtime{
		repository: config.Repository, clientFactory: config.ClientFactory,
		grantIssuer: config.GrantIssuer, enabled: config.Enabled, now: config.Now, newID: config.NewID,
		projectStream: config.ProjectStream, publishStream: config.PublishStream,
		fallbackAt: make(map[string]time.Time),
	}, nil
}

func InitialPromptTurnID(sessionID string) string { return initialTurnPrefix + sessionID }

// Launch durably reserves the managed identity. Paid provider work begins
// only in StartExecution, after the caller persists the returned execution ID.
func (r *Runtime) Launch(ctx context.Context, spec runtime.LaunchSpec) (runtime.ExecutionRef, error) {
	value, ok := spec.Metadata[LaunchMetadataKey].(LaunchInput)
	if !ok || value.Binding == nil || value.Operation == nil {
		return runtime.ExecutionRef{}, errors.New("cursor cloud launch input is missing")
	}
	if value.Binding.ProviderKind != "cursor_cloud" || value.Binding.ExecutorID == "" ||
		value.Operation.PromptTurnID != InitialPromptTurnID(value.Binding.SessionID) {
		return runtime.ExecutionRef{}, errors.New("cursor cloud launch identity is invalid")
	}
	owner := r.newID()
	leaseUntil := r.now().Add(leaseDuration)
	binding, _, _, err := r.repository.ReserveManagedAgentStart(ctx, value.Binding, value.Operation, owner, leaseUntil)
	if err != nil {
		return runtime.ExecutionRef{}, fmt.Errorf("reserve Cursor Cloud launch: %w", err)
	}
	return runtime.ExecutionRef{ID: binding.ExecutionID, SessionID: binding.SessionID, StartedAt: r.now()}, nil
}

func (r *Runtime) Start(ctx context.Context, spec runtime.LaunchSpec) (runtime.ExecutionRef, error) {
	ref, err := r.Launch(ctx, spec)
	if err != nil {
		return runtime.ExecutionRef{}, err
	}
	if err := r.StartExecution(ctx, ref.ID); err != nil {
		return runtime.ExecutionRef{}, err
	}
	return ref, nil
}

func (r *Runtime) StartExecution(ctx context.Context, executionID string) error {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load Cursor Cloud binding: %w", err)
	}
	operation, err := r.repository.GetManagedAgentOperationByPromptTurnID(ctx, InitialPromptTurnID(binding.SessionID))
	if err != nil {
		return fmt.Errorf("load Cursor Cloud create operation: %w", err)
	}
	return r.dispatchCreate(ctx, binding, operation)
}

func (r *Runtime) dispatchCreate(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) error {
	switch operation.State {
	case models.ManagedAgentSubmissionAccepted, models.ManagedAgentSubmissionSucceeded:
		return nil
	case models.ManagedAgentSubmissionUnknown:
		return r.reconcileCreate(ctx, binding, operation)
	case models.ManagedAgentSubmissionRejected, models.ManagedAgentSubmissionFailed, models.ManagedAgentSubmissionCancelled:
		return fmt.Errorf("cursor cloud create operation is %s", operation.State)
	case models.ManagedAgentSubmissionSubmitting:
		return r.reconcileCreate(ctx, binding, operation)
	case models.ManagedAgentSubmissionReserved:
		if !r.enabled() {
			return errors.New("cursor cloud is disabled for new dispatches")
		}
	default:
		return fmt.Errorf("cursor cloud create operation has invalid state %q", operation.State)
	}
	return r.submitCreate(ctx, binding, operation)
}

func (r *Runtime) submitCreate(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) error {
	leaseBinding, owner, err := r.ensureLease(ctx, binding, operation)
	if err != nil {
		return err
	}
	grant, err := r.grantIssuer.Issue(ctx, leaseBinding, operation)
	if err != nil {
		return fmt.Errorf("issue Cursor Cloud MCP grant: %w", err)
	}
	if strings.TrimSpace(grant.URL) == "" || strings.TrimSpace(grant.Token) == "" {
		return errors.New("cursor cloud MCP grant is incomplete")
	}
	operation, err = r.updateOperation(ctx, leaseBinding, operation, owner, models.ManagedAgentSubmissionSubmitting, "", "")
	if err != nil {
		return fmt.Errorf("record Cursor Cloud create dispatch: %w", err)
	}
	client, err := r.clientFactory(ctx, leaseBinding)
	if err != nil {
		return r.rejectOperation(ctx, leaseBinding, operation, owner, err)
	}
	request := buildCreateRequest(leaseBinding, operation, grant)
	response, err := client.CreateAgent(ctx, request)
	if err != nil {
		if errors.Is(err, provider.ErrOutcomeUnknown) {
			return r.reconcileCreate(ctx, leaseBinding, operation)
		}
		return r.rejectOperation(ctx, leaseBinding, operation, owner, err)
	}
	return r.acceptOperation(ctx, leaseBinding, operation, owner, response.Run.ID)
}

func buildCreateRequest(binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, grant MCPGrant) provider.CreateAgentRequest {
	snapshot := operation.RequestSnapshot
	return provider.CreateAgentRequest{
		AgentID:      binding.RemoteAgentID,
		Prompt:       provider.Prompt{Text: snapshot.Prompt},
		Model:        &provider.ModelSelection{ID: snapshot.Model},
		Repos:        []provider.Repository{{URL: snapshot.RepositoryURL, StartingRef: snapshot.StartingRef}},
		AutoCreatePR: snapshot.AutoCreatePR,
		MCPServers:   []provider.MCPServer{{Name: "kandev", Type: "http", URL: grant.URL, Headers: map[string]string{"Authorization": "Bearer " + grant.Token}}},
	}
}

func (r *Runtime) reconcileCreate(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) error {
	client, err := r.clientFactory(ctx, binding)
	if err != nil {
		return r.markUnknown(ctx, binding, operation, err)
	}
	agent, err := client.GetAgent(ctx, binding.RemoteAgentID)
	if err != nil || agent.LatestRunID == "" {
		if err == nil {
			err = errors.New("cursor cloud agent has no confirmed initial run")
		}
		return r.markUnknown(ctx, binding, operation, err)
	}
	run, err := client.GetRun(ctx, binding.RemoteAgentID, agent.LatestRunID)
	if err != nil || run.AgentID != binding.RemoteAgentID || run.ID == "" {
		if err == nil {
			err = errors.New("cursor cloud initial run identity is invalid")
		}
		return r.markUnknown(ctx, binding, operation, err)
	}
	if operation.State == models.ManagedAgentSubmissionSubmitting || operation.State == models.ManagedAgentSubmissionUnknown {
		leaseBinding, owner, leaseErr := r.ensureLease(ctx, binding, operation)
		if leaseErr != nil {
			return leaseErr
		}
		binding = leaseBinding
		return r.acceptOperation(ctx, binding, operation, owner, run.ID)
	}
	return nil
}

func (r *Runtime) Resume(ctx context.Context, executionID string, prompt string) error {
	return r.ResumeWithTurnID(ctx, executionID, r.newID(), prompt)
}

func (r *Runtime) ResumeWithTurnID(ctx context.Context, executionID, turnID, prompt string) error {
	if !r.enabled() {
		return errors.New("cursor cloud is disabled for new dispatches")
	}
	if strings.TrimSpace(turnID) == "" || strings.TrimSpace(prompt) == "" {
		return errors.New("cursor cloud follow-up identity and prompt are required")
	}
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load Cursor Cloud binding: %w", err)
	}
	if existing, getErr := r.repository.GetManagedAgentOperationByPromptTurnID(ctx, turnID); getErr == nil {
		if existing.BindingID != binding.ID || existing.Kind != models.ManagedAgentOperationFollowup ||
			existing.RequestDigest != digestRequest(prompt, turnID, binding.Launch) {
			return errors.New("cursor cloud follow-up turn identity conflicts with another operation")
		}
		return resultForOperation(existing)
	} else if !errors.Is(getErr, repository.ErrManagedAgentOperationNotFound) {
		return fmt.Errorf("load Cursor Cloud follow-up identity: %w", getErr)
	}
	operation := &models.ManagedAgentOperation{
		ID: r.newID(), BindingID: binding.ID, PromptTurnID: turnID,
		Kind:          models.ManagedAgentOperationFollowup,
		RequestDigest: digestRequest(prompt, turnID, binding.Launch),
		RequestSnapshot: models.ManagedAgentRequestSnapshot{
			Prompt: prompt, TurnID: turnID, MCPMode: "task", RepositoryURL: binding.Launch.RepositoryURL, StartingRef: binding.Launch.StartingRef,
			Model: binding.Launch.Model, CallbackURL: binding.Launch.CallbackURL, AutoCreatePR: binding.Launch.AutoCreatePR,
		},
	}
	owner := r.newID()
	reservedBinding, reservedOperation, _, err := r.repository.ReserveManagedAgentOperation(ctx, operation, binding.Revision, owner, r.now().Add(leaseDuration))
	if err != nil {
		return fmt.Errorf("reserve Cursor Cloud follow-up: %w", err)
	}
	return r.dispatchFollowup(ctx, reservedBinding, reservedOperation, owner)
}

// ResumeReservedFollowup dispatches a follow-up whose durable journal still
// says reserved. The provider call cannot have started before that transition.
func (r *Runtime) ResumeReservedFollowup(ctx context.Context, executionID string) error {
	if !r.enabled() {
		return errors.New("cursor cloud is disabled for new dispatches")
	}
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return err
	}
	if operation.Kind != models.ManagedAgentOperationFollowup || operation.State != models.ManagedAgentSubmissionReserved {
		return nil
	}
	leaseBinding, owner, err := r.ensureLease(ctx, binding, operation)
	if err != nil {
		return err
	}
	return r.dispatchFollowup(ctx, leaseBinding, operation, owner)
}

// MarkFollowupSubmissionUnknownAfterRestart converts an interrupted submit
// into visible uncertainty without sending another provider request.
func (r *Runtime) MarkFollowupSubmissionUnknownAfterRestart(ctx context.Context, executionID string) error {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return err
	}
	if operation.Kind != models.ManagedAgentOperationFollowup || operation.State != models.ManagedAgentSubmissionSubmitting || operation.RemoteRunID != "" {
		return nil
	}
	leaseBinding, owner, err := r.ensureLease(ctx, binding, operation)
	if err != nil {
		return err
	}
	if _, err := r.updateOperation(ctx, leaseBinding, operation, owner, models.ManagedAgentSubmissionUnknown, "",
		"Backend restarted during provider submission; remote outcome is unknown."); err != nil {
		return fmt.Errorf("record interrupted Cursor Cloud follow-up: %w", err)
	}
	recordDispatch(operation.Kind, "unknown")
	recordSubmissionUnknown(operation.Kind, "crash_recovery")
	return nil
}

func (r *Runtime) dispatchFollowup(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, owner string) error {
	if operation.State != models.ManagedAgentSubmissionReserved {
		return resultForOperation(operation)
	}
	if !r.enabled() {
		return errors.New("cursor cloud is disabled for new dispatches")
	}
	client, err := r.clientFactory(ctx, binding)
	if err != nil {
		return r.rejectOperation(ctx, binding, operation, owner, err)
	}
	agent, err := client.GetAgent(ctx, binding.RemoteAgentID)
	if err != nil {
		return r.rejectOperation(ctx, binding, operation, owner, err)
	}
	if agent.ID != binding.RemoteAgentID {
		return r.rejectOperation(ctx, binding, operation, owner, errors.New("cursor cloud agent response identity is invalid"))
	}
	grant, err := r.grantIssuer.Issue(ctx, binding, operation)
	if err != nil {
		return r.rejectOperation(ctx, binding, operation, owner, fmt.Errorf("issue Cursor Cloud MCP grant: %w", err))
	}
	operation, err = r.updateFollowupPreSubmitRun(ctx, binding, operation, owner, agent.LatestRunID)
	if err != nil {
		return fmt.Errorf("record Cursor Cloud follow-up dispatch identity: %w", err)
	}
	run, err := client.CreateRun(ctx, binding.RemoteAgentID, provider.CreateRunRequest{
		Prompt: provider.Prompt{Text: operation.RequestSnapshot.Prompt},
		MCPServers: []provider.MCPServer{{Name: "kandev", Type: "http", URL: grant.URL,
			Headers: map[string]string{"Authorization": "Bearer " + grant.Token}}},
	})
	if err != nil {
		if errors.Is(err, provider.ErrOutcomeUnknown) {
			return r.markUnknown(ctx, binding, operation, err)
		}
		return r.rejectOperation(ctx, binding, operation, owner, err)
	}
	if run.AgentID != binding.RemoteAgentID || run.ID == "" {
		return r.markUnknown(ctx, binding, operation, errors.New("cursor cloud follow-up response identity is invalid"))
	}
	return r.acceptOperation(ctx, binding, operation, owner, run.ID)
}

func (r *Runtime) ensureLease(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation) (*models.ManagedAgentBinding, string, error) {
	if binding.DispatchOwner != "" && binding.DispatchLeaseUntil != nil && binding.DispatchLeaseUntil.After(r.now()) {
		return binding, binding.DispatchOwner, nil
	}
	owner := r.newID()
	claimed, err := r.repository.ClaimManagedAgentDispatchLease(ctx, binding.ID, operation.ID, binding.Revision, owner, r.now().Add(leaseDuration))
	if err != nil {
		return nil, "", fmt.Errorf("claim Cursor Cloud dispatch lease: %w", err)
	}
	return claimed, owner, nil
}

func (r *Runtime) updateOperation(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, owner string, state models.ManagedAgentSubmissionState, runID, safeError string) (*models.ManagedAgentOperation, error) {
	return r.repository.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: binding.Revision, LeaseOwner: owner,
		State: state, RemoteRunID: runID, SanitizedError: safeError,
	})
}

func (r *Runtime) updateFollowupPreSubmitRun(
	ctx context.Context,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	owner, preSubmitRunID string,
) (*models.ManagedAgentOperation, error) {
	return r.repository.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision,
		ExpectedBindingRevision: binding.Revision, LeaseOwner: owner,
		State: models.ManagedAgentSubmissionSubmitting, PreSubmitRunID: preSubmitRunID,
	})
}

func (r *Runtime) acceptOperation(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, owner, runID string) error {
	if _, err := r.updateOperation(ctx, binding, operation, owner, models.ManagedAgentSubmissionAccepted, runID, ""); err != nil {
		return fmt.Errorf("persist accepted Cursor Cloud run: %w", err)
	}
	if operation.State != models.ManagedAgentSubmissionAccepted {
		recordDispatch(operation.Kind, "accepted")
	}
	return nil
}

func (r *Runtime) markUnknown(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, cause error) error {
	owner := binding.DispatchOwner
	if operation.State == models.ManagedAgentSubmissionSubmitting {
		var err error
		binding, owner, err = r.ensureLease(ctx, binding, operation)
		if err != nil {
			return errors.Join(provider.ErrOutcomeUnknown, err)
		}
	}
	_, updateErr := r.updateOperation(ctx, binding, operation, owner, models.ManagedAgentSubmissionUnknown, "", sanitizeError(cause))
	if updateErr == nil && operation.State != models.ManagedAgentSubmissionUnknown {
		recordDispatch(operation.Kind, "unknown")
		reason := "transport"
		if errors.Is(cause, provider.ErrOutcomeUnknown) {
			reason = "timeout"
		}
		recordSubmissionUnknown(operation.Kind, reason)
	}
	return errors.Join(provider.ErrOutcomeUnknown, updateErr)
}

func (r *Runtime) rejectOperation(ctx context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, owner string, cause error) error {
	_, updateErr := r.updateOperation(ctx, binding, operation, owner, models.ManagedAgentSubmissionRejected, "", sanitizeError(cause))
	if updateErr == nil && operation.State != models.ManagedAgentSubmissionRejected {
		recordDispatch(operation.Kind, "rejected")
	}
	return errors.Join(cause, updateErr)
}

func resultForOperation(operation *models.ManagedAgentOperation) error {
	if operation == nil {
		return errors.New("cursor cloud operation is missing")
	}
	switch operation.State {
	case models.ManagedAgentSubmissionAccepted, models.ManagedAgentSubmissionSucceeded:
		return nil
	case models.ManagedAgentSubmissionUnknown, models.ManagedAgentSubmissionSubmitting:
		return provider.ErrOutcomeUnknown
	case models.ManagedAgentSubmissionRejected, models.ManagedAgentSubmissionFailed, models.ManagedAgentSubmissionCancelled:
		return fmt.Errorf("cursor cloud operation is %s", operation.State)
	default:
		return fmt.Errorf("cursor cloud operation is %s", operation.State)
	}
}

func (r *Runtime) Stop(ctx context.Context, executionID, reason string) error {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return err
	}
	_ = reason
	return r.cancelOperation(ctx, binding, operation)
}

// CancelActive interrupts the latest remote turn and retains its conversation.
// It returns only after a provider read confirms a terminal outcome.
func (r *Runtime) CancelActive(ctx context.Context, executionID string) error {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return err
	}
	operation, err := r.repository.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		return err
	}
	return r.cancelOperation(ctx, binding, operation)
}

func githubRepositoryIdentity(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	path := strings.TrimSuffix(strings.Trim(strings.TrimSpace(parsed.Path), "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	return strings.ToLower(parts[0] + "/" + parts[1]), true
}

func validatedPullRequestURL(raw, repositoryIdentity string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || !strings.EqualFold(parsed.Host, "github.com") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || strings.ToLower(parts[0]+"/"+parts[1]) != repositoryIdentity || parts[2] != "pull" || parts[3] == "" {
		return ""
	}
	for _, char := range parts[3] {
		if char < '0' || char > '9' {
			return ""
		}
	}
	return "https://github.com/" + strings.Join(parts, "/")
}

func safeCursorAgentURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") ||
		(!strings.EqualFold(parsed.Host, "cursor.com") && !strings.EqualFold(parsed.Host, "www.cursor.com")) ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	return parsed.String()
}

func terminalEventType(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FINISHED":
		return "complete"
	case runStatusCancelled:
		return "cancelled"
	case runStatusError, runStatusExpired:
		return "error"
	default:
		return ""
	}
}

func (r *Runtime) GetExecution(ctx context.Context, executionID string) (*runtime.Execution, error) {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return &runtime.Execution{
		ID: binding.ExecutionID, SessionID: binding.SessionID, TaskID: binding.TaskID,
		AgentProfileID: binding.ExecutorProfileID, Status: v1.AgentStatusRunning,
		StartedAt: binding.CreatedAt, Metadata: map[string]interface{}{"runtime": "cursor_cloud", "remote_agent_id": binding.RemoteAgentID},
	}, nil
}

func (*Runtime) SubscribeEvents(context.Context, string) (<-chan runtime.Event, error) {
	return nil, runtime.ErrUnsupported
}

func (r *Runtime) SetMcpMode(ctx context.Context, executionID, mode string) error {
	binding, err := r.repository.GetManagedAgentBindingByExecution(ctx, executionID)
	if err != nil {
		return err
	}
	operation, err := r.repository.GetManagedAgentOperationByPromptTurnID(ctx, InitialPromptTurnID(binding.SessionID))
	if err != nil {
		return err
	}
	if mode == operation.RequestSnapshot.MCPMode {
		return nil
	}
	return runtime.ErrUnsupported
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	if len(message) > 256 {
		message = message[:256]
	}
	return message
}

func digestRequest(prompt, turnID string, launch models.ManagedAgentLaunchSnapshot) string {
	return fmt.Sprintf("%x", sha256Sum([]byte(strings.Join([]string{prompt, turnID, "task", launch.RepositoryURL, launch.StartingRef, launch.Model, launch.CallbackURL, fmt.Sprint(launch.AutoCreatePR)}, "\x00"))))
}

func sha256Sum(value []byte) [32]byte { return sha256.Sum256(value) }
