package cursorcloud

import (
	"context"
	"errors"
	"time"

	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type memoryRepository struct {
	binding          *models.ManagedAgentBinding
	operation        *models.ManagedAgentOperation
	operations       map[string]*models.ManagedAgentOperation
	getOperationErr  error
	streamCheckpoint *models.ManagedAgentStreamCheckpoint
	streamEvents     map[string]bool
	streamMessages   []*models.Message
}

func newMemoryRepository() *memoryRepository { return &memoryRepository{} }

func (r *memoryRepository) ReserveManagedAgentStart(_ context.Context, binding *models.ManagedAgentBinding, operation *models.ManagedAgentOperation, owner string, until time.Time) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	if r.binding != nil {
		return cloneBinding(r.binding), cloneOperation(r.operation), true, nil
	}
	r.binding = cloneBinding(binding)
	r.binding.Revision = 1
	r.binding.DispatchGeneration = 1
	r.binding.DispatchOwner = owner
	r.binding.DispatchLeaseUntil = &until
	r.operation = cloneOperation(operation)
	r.operations = map[string]*models.ManagedAgentOperation{operation.PromptTurnID: r.operation}
	r.operation.Revision = 1
	r.operation.DispatchGeneration = 1
	r.operation.State = models.ManagedAgentSubmissionReserved
	return cloneBinding(r.binding), cloneOperation(r.operation), false, nil
}

func (r *memoryRepository) ReserveManagedAgentOperation(_ context.Context, operation *models.ManagedAgentOperation, expectedRevision int64, owner string, until time.Time) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, bool, error) {
	if existing := r.operations[operation.PromptTurnID]; existing != nil {
		if existing.BindingID != operation.BindingID || existing.Kind != operation.Kind || existing.RequestDigest != operation.RequestDigest {
			return nil, nil, false, repository.ErrManagedAgentOperationConflict
		}
		return cloneBinding(r.binding), cloneOperation(existing), true, nil
	}
	if r.binding == nil || r.binding.Revision != expectedRevision || r.operation == nil {
		return nil, nil, false, errors.New("active operation")
	}
	if operation.RetryAcknowledgesOperationID != "" {
		if !operation.DuplicationRiskAcknowledged || r.operation.ID != operation.RetryAcknowledgesOperationID ||
			(r.operation.State != models.ManagedAgentSubmissionUnknown && r.operation.State != models.ManagedAgentSubmissionSubmitting) ||
			r.operation.Kind != operation.Kind {
			return nil, nil, false, errors.New("retry acknowledgment does not match the unknown operation")
		}
		r.operation.State = models.ManagedAgentSubmissionRetryAcked
		r.operation.Revision++
		r.operation.SanitizedError = "User acknowledged that retrying may create duplicate remote work."
	}
	if r.operation.State == models.ManagedAgentSubmissionAccepted || r.operation.State == models.ManagedAgentSubmissionReserved ||
		r.operation.State == models.ManagedAgentSubmissionSubmitting || r.operation.State == models.ManagedAgentSubmissionUnknown {
		return nil, nil, false, errors.New("active operation")
	}
	r.binding.Revision++
	r.binding.DispatchGeneration++
	r.binding.DispatchOwner = owner
	r.binding.DispatchLeaseUntil = &until
	r.operation = cloneOperation(operation)
	if r.operations == nil {
		r.operations = make(map[string]*models.ManagedAgentOperation)
	}
	r.operations[operation.PromptTurnID] = r.operation
	r.operation.Revision = 1
	r.operation.DispatchGeneration = r.binding.DispatchGeneration
	r.operation.State = models.ManagedAgentSubmissionReserved
	return cloneBinding(r.binding), cloneOperation(r.operation), false, nil
}

func (r *memoryRepository) ClaimManagedAgentDispatchLease(_ context.Context, bindingID, operationID string, expectedRevision int64, owner string, until time.Time) (*models.ManagedAgentBinding, error) {
	if r.binding == nil || r.binding.ID != bindingID || r.operation == nil || r.operation.ID != operationID || r.binding.Revision != expectedRevision {
		return nil, errors.New("revision conflict")
	}
	r.binding.Revision++
	r.binding.DispatchOwner = owner
	r.binding.DispatchLeaseUntil = &until
	return cloneBinding(r.binding), nil
}

func (r *memoryRepository) ClaimManagedAgentCancellationLease(ctx context.Context, bindingID, operationID string, expectedRevision int64, owner string, until time.Time) (*models.ManagedAgentBinding, error) {
	return r.ClaimManagedAgentDispatchLease(ctx, bindingID, operationID, expectedRevision, owner, until)
}

func (r *memoryRepository) GetManagedAgentBindingBySession(_ context.Context, sessionID string) (*models.ManagedAgentBinding, error) {
	if r.binding == nil || r.binding.SessionID != sessionID {
		return nil, repository.ErrManagedAgentBindingNotFound
	}
	return cloneBinding(r.binding), nil
}

func (r *memoryRepository) GetManagedAgentBindingByExecution(_ context.Context, executionID string) (*models.ManagedAgentBinding, error) {
	if r.binding == nil || r.binding.ExecutionID != executionID {
		return nil, repository.ErrManagedAgentBindingNotFound
	}
	return cloneBinding(r.binding), nil
}

func (r *memoryRepository) GetManagedAgentOperationByPromptTurnID(_ context.Context, turnID string) (*models.ManagedAgentOperation, error) {
	if r.getOperationErr != nil {
		return nil, r.getOperationErr
	}
	operation := r.operations[turnID]
	if operation == nil {
		return nil, repository.ErrManagedAgentOperationNotFound
	}
	return cloneOperation(operation), nil
}

func (r *memoryRepository) GetManagedAgentOperation(_ context.Context, operationID string) (*models.ManagedAgentOperation, error) {
	if r.operation == nil || r.operation.ID != operationID {
		return nil, repository.ErrManagedAgentOperationNotFound
	}
	return cloneOperation(r.operation), nil
}

func (r *memoryRepository) GetManagedAgentLatestOperation(_ context.Context, bindingID string) (*models.ManagedAgentOperation, error) {
	if r.binding == nil || r.binding.ID != bindingID || r.operation == nil {
		return nil, repository.ErrManagedAgentOperationNotFound
	}
	return cloneOperation(r.operation), nil
}

func (r *memoryRepository) ListActiveManagedAgentBindings(context.Context) ([]*models.ManagedAgentBinding, error) {
	if r.binding == nil || r.operation == nil || (!models.ManagedAgentOperationActive(r.operation.State) && !r.operation.CompletionPending) {
		return []*models.ManagedAgentBinding{}, nil
	}
	return []*models.ManagedAgentBinding{cloneBinding(r.binding)}, nil
}

func (r *memoryRepository) AcknowledgeManagedAgentCompletion(_ context.Context, operationID string) error {
	if r.operation == nil || r.operation.ID != operationID {
		return repository.ErrManagedAgentOperationNotFound
	}
	r.operation.CompletionPending = false
	r.operation.Revision++
	return nil
}

func (r *memoryRepository) CompareAndSwapManagedAgentOperation(_ context.Context, update models.ManagedAgentOperationUpdate) (*models.ManagedAgentOperation, error) {
	if r.getOperationErr != nil {
		return nil, r.getOperationErr
	}
	if r.binding == nil || r.operation == nil || r.operation.ID != update.OperationID ||
		r.operation.Revision != update.ExpectedRevision || r.binding.Revision != update.ExpectedBindingRevision ||
		r.binding.DispatchOwner != update.LeaseOwner {
		return nil, errors.New("revision conflict")
	}
	r.operation.State = update.State
	r.operation.Revision++
	if update.RemoteRunID != "" {
		r.operation.RemoteRunID = update.RemoteRunID
	}
	r.operation.SanitizedError = update.SanitizedError
	if update.PreSubmitRunID != "" {
		r.operation.PreSubmitRunID = update.PreSubmitRunID
	}
	if update.State == models.ManagedAgentSubmissionSubmitting && r.operation.DispatchStartedAt == nil {
		now := time.Now().UTC()
		r.operation.DispatchStartedAt = &now
	}
	if update.ResultSnapshot != nil {
		r.operation.ResultSnapshot = *update.ResultSnapshot
	}
	if update.CompletionPending != nil {
		r.operation.CompletionPending = *update.CompletionPending
	}
	r.operations[r.operation.PromptTurnID] = r.operation
	if update.State == models.ManagedAgentSubmissionAccepted || update.State == models.ManagedAgentSubmissionUnknown ||
		!models.ManagedAgentOperationActive(update.State) {
		r.binding.Revision++
		r.binding.DispatchOwner = ""
		r.binding.DispatchLeaseUntil = nil
	}
	return cloneOperation(r.operation), nil
}

func (r *memoryRepository) GetManagedAgentStreamCheckpoint(_ context.Context, bindingID, runID string) (*models.ManagedAgentStreamCheckpoint, error) {
	if r.streamCheckpoint == nil || r.streamCheckpoint.BindingID != bindingID || r.streamCheckpoint.RemoteRunID != runID {
		return nil, repository.ErrManagedAgentStreamNotFound
	}
	checkpoint := *r.streamCheckpoint
	return &checkpoint, nil
}

func (r *memoryRepository) CommitManagedAgentStreamEvent(_ context.Context, event models.ManagedAgentStreamEvent) (bool, error) {
	if r.operation == nil || r.binding == nil || event.BindingID != r.binding.ID || event.OperationID != r.operation.ID ||
		event.RemoteRunID != r.operation.RemoteRunID || event.DispatchGeneration != r.operation.DispatchGeneration {
		return false, errors.New("stream event identity conflict")
	}
	if r.streamEvents == nil {
		r.streamEvents = make(map[string]bool)
	}
	key := event.BindingID + "\x00" + event.RemoteRunID + "\x00" + event.EventID + "\x00" + event.EventType
	if event.EventID != "" && r.streamEvents[key] {
		return false, nil
	}
	if event.EventID != "" {
		r.streamEvents[key] = true
	}
	if event.Message != nil {
		r.upsertStreamMessage(event.Message, event.AppendMessage)
	}
	checkpoint := &models.ManagedAgentStreamCheckpoint{
		BindingID: event.BindingID, RemoteRunID: event.RemoteRunID,
		DispatchGeneration: event.DispatchGeneration,
		HistoryGap:         event.HistoryGap,
	}
	if r.streamCheckpoint != nil {
		*checkpoint = *r.streamCheckpoint
	}
	if event.EventID != "" && event.EventType != "terminal_result_readback" {
		checkpoint.LastEventID, checkpoint.Cursor = event.EventID, event.Cursor
	}
	checkpoint.AssistantMessageStarted = checkpoint.AssistantMessageStarted || event.AssistantMessageStarted
	if event.TerminalEventType != "" {
		checkpoint.TerminalEventType = event.TerminalEventType
	}
	checkpoint.HistoryGap = checkpoint.HistoryGap || event.HistoryGap
	r.streamCheckpoint = checkpoint
	return true, nil
}

func (r *memoryRepository) upsertStreamMessage(message *models.Message, appendMessage bool) {
	for _, saved := range r.streamMessages {
		if saved.ID != message.ID {
			continue
		}
		if appendMessage {
			saved.Content += message.Content
		} else {
			*saved = *message
		}
		return
	}
	copy := *message
	r.streamMessages = append(r.streamMessages, &copy)
}

func cloneBinding(binding *models.ManagedAgentBinding) *models.ManagedAgentBinding {
	if binding == nil {
		return nil
	}
	copy := *binding
	if binding.DispatchLeaseUntil != nil {
		leaseUntil := *binding.DispatchLeaseUntil
		copy.DispatchLeaseUntil = &leaseUntil
	}
	return &copy
}

func cloneOperation(operation *models.ManagedAgentOperation) *models.ManagedAgentOperation {
	if operation == nil {
		return nil
	}
	copy := *operation
	copy.RequestSnapshot = operation.RequestSnapshot
	return &copy
}

type fakeProvider struct {
	createCalls    int
	createRunCalls int
	cancelRunCalls int
	createResponse provider.CreateAgentResponse
	createErr      error
	createRun      provider.Run
	createRunErr   error
	agent          provider.Agent
	getAgentErr    error
	getRun         provider.Run
	getRunErr      error
	getRunStatus   string
	getRunCalls    int
	cancelRunErr   error
	streamEvents   []provider.StreamEvent
	streamErr      error
	streamCalls    int
	streamLastIDs  []string
	runs           []provider.Run
	listRunsErr    error
}

func (p *fakeProvider) CreateAgent(_ context.Context, _ provider.CreateAgentRequest) (provider.CreateAgentResponse, error) {
	p.createCalls++
	return p.createResponse, p.createErr
}

func (p *fakeProvider) CreateRun(_ context.Context, _ string, _ provider.CreateRunRequest) (provider.Run, error) {
	p.createRunCalls++
	if p.createRunErr != nil {
		return provider.Run{}, p.createRunErr
	}
	return p.createRun, nil
}

func (p *fakeProvider) GetAgent(_ context.Context, agentID string) (provider.Agent, error) {
	if p.agent.ID != "" || p.agent.URL != "" || p.agent.LatestRunID != "" {
		agent := p.agent
		if agent.ID == "" {
			agent.ID = agentID
		}
		return agent, p.getAgentErr
	}
	if p.createResponse.Agent.ID == agentID {
		return p.createResponse.Agent, p.getAgentErr
	}
	return provider.Agent{ID: agentID}, p.getAgentErr
}

func (p *fakeProvider) GetRun(_ context.Context, agentID, runID string) (provider.Run, error) {
	p.getRunCalls++
	if p.getRun.ID == "" {
		p.getRun.ID, p.getRun.AgentID = runID, agentID
	}
	if p.getRunStatus != "" {
		p.getRun.Status = p.getRunStatus
	}
	return p.getRun, p.getRunErr
}

func (p *fakeProvider) CancelRun(context.Context, string, string) error {
	p.cancelRunCalls++
	return p.cancelRunErr
}

func (p *fakeProvider) ListRuns(_ context.Context, agentID string, _ int, _ string) (provider.RunPage, error) {
	if p.listRunsErr != nil {
		return provider.RunPage{}, p.listRunsErr
	}
	page := provider.RunPage{}
	for _, run := range p.runs {
		if run.AgentID == agentID {
			page.Items = append(page.Items, run)
		}
	}
	return page, nil
}

func (p *fakeProvider) StreamRun(_ context.Context, _, _ string, lastEventID string, handle func(provider.StreamEvent) error) (provider.StreamResult, error) {
	p.streamCalls++
	p.streamLastIDs = append(p.streamLastIDs, lastEventID)
	for _, event := range p.streamEvents {
		if event.ID != "" && lastEventID != "" && event.ID <= lastEventID {
			continue
		}
		if err := handle(event); err != nil {
			return provider.StreamResult{}, err
		}
	}
	return provider.StreamResult{}, p.streamErr
}
