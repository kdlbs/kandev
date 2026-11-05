package hostutility

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const (
	antigravityLoginTimeout = 10 * time.Minute
	antigravityLoginPoll    = 500 * time.Millisecond
)

// AntigravityLoginSnapshot is the operator-visible state of one sign-in
// attempt. The authorization URL is transient and never persisted.
type AntigravityLoginSnapshot struct {
	AttemptID        string    `json:"attempt_id"`
	State            string    `json:"state"`
	ExpiresAt        time.Time `json:"expires_at"`
	AuthorizationURL string    `json:"authorization_url,omitempty"`
	ErrorCode        string    `json:"error_code,omitempty"`
	Error            string    `json:"error,omitempty"`
}

type antigravityLoginAttempt struct {
	mu        sync.Mutex
	id        string
	state     string
	expiresAt time.Time
	url       string
	errCode   string
	err       string
	terminal  bool
	cancel    context.CancelFunc
	done      chan struct{}
	inst      *instance
}

func (a *antigravityLoginAttempt) snapshot() AntigravityLoginSnapshot {
	a.mu.Lock()
	expired := false
	if isActiveLoginState(a.state) && time.Now().After(a.expiresAt) {
		a.state = "expired"
		a.terminal = true
		a.url = ""
		expired = true
	}
	snap := AntigravityLoginSnapshot{
		AttemptID:        a.id,
		State:            a.state,
		ExpiresAt:        a.expiresAt,
		AuthorizationURL: a.url,
		ErrorCode:        a.errCode,
		Error:            a.err,
	}
	a.mu.Unlock()
	if expired && a.cancel != nil {
		a.cancel()
	}
	return snap
}

func (a *antigravityLoginAttempt) update(state, url, errCode, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.terminal {
		return
	}
	a.state = state
	if url != "" {
		a.url = url
	} else if state == "checking" {
		a.url = ""
	}
	a.errCode = errCode
	a.err = message
}

func (a *antigravityLoginAttempt) finish(state, errCode, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.terminal {
		return
	}
	a.state = state
	a.terminal = true
	a.url = ""
	a.errCode = errCode
	a.err = message
}

func (a *antigravityLoginAttempt) isDone() bool {
	select {
	case <-a.done:
		return true
	default:
		return false
	}
}

func isActiveLoginState(state string) bool {
	switch state {
	case "starting", "waiting_for_browser", "checking":
		return true
	}
	return false
}

// StartAntigravityLogin starts or returns the active bounded sign-in attempt.
func (m *Manager) StartAntigravityLogin(ctx context.Context) (AntigravityLoginSnapshot, error) {
	ag, ok := m.registry.Get(agents.AntigravityACPAgentID)
	if !ok {
		return AntigravityLoginSnapshot{}, errors.New("antigravity agent is not registered")
	}
	if _, err := ag.IsInstalled(ctx); err != nil {
		return AntigravityLoginSnapshot{}, err
	}
	if _, ok := ag.(agents.LocalRuntimeAgent); !ok {
		return AntigravityLoginSnapshot{}, errors.New("antigravity local runtime is unavailable")
	}
	m.loginMu.Lock()
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()
	if stopped {
		m.loginMu.Unlock()
		return AntigravityLoginSnapshot{}, errManagerStopped
	}
	if current := m.loginAttempt; current != nil {
		snap := current.snapshot()
		if isActiveLoginState(snap.State) || !current.isDone() {
			m.loginMu.Unlock()
			return snap, nil
		}
	}
	attemptCtx, cancel := context.WithTimeout(context.Background(), antigravityLoginTimeout)
	attempt := &antigravityLoginAttempt{
		id:        uuid.NewString(),
		state:     "starting",
		expiresAt: time.Now().Add(antigravityLoginTimeout),
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	m.loginAttempt = attempt
	m.loginMu.Unlock()
	go m.runAntigravityLogin(attemptCtx, attempt)
	return attempt.snapshot(), nil
}

// GetAntigravityLogin returns the latest or named attempt snapshot.
func (m *Manager) GetAntigravityLogin(ctx context.Context, attemptID string) (AntigravityLoginSnapshot, bool) {
	if err := ctx.Err(); err != nil {
		return AntigravityLoginSnapshot{}, false
	}
	m.loginMu.Lock()
	attempt := m.loginAttempt
	m.loginMu.Unlock()
	if attempt == nil {
		return AntigravityLoginSnapshot{}, false
	}
	snap := attempt.snapshot()
	if attemptID != "" && snap.AttemptID != attemptID {
		return AntigravityLoginSnapshot{}, false
	}
	return snap, true
}

// CancelAntigravityLogin cancels the named active attempt idempotently.
func (m *Manager) CancelAntigravityLogin(ctx context.Context, attemptID string) (AntigravityLoginSnapshot, bool) {
	m.loginMu.Lock()
	attempt := m.loginAttempt
	m.loginMu.Unlock()
	if attempt == nil {
		return AntigravityLoginSnapshot{}, false
	}
	snap := attempt.snapshot()
	if attemptID != "" && snap.AttemptID != attemptID {
		return AntigravityLoginSnapshot{}, false
	}
	attempt.finish("cancelled", "cancelled", "Antigravity sign-in was cancelled")
	attempt.cancel()
	select {
	case <-attempt.done:
	case <-ctx.Done():
	}
	return attempt.snapshot(), true
}

func (m *Manager) runAntigravityLogin(ctx context.Context, attempt *antigravityLoginAttempt) {
	defer close(attempt.done)
	defer attempt.cancel()
	inst, ia, err := m.createAntigravityLoginInstance(ctx, attempt)
	if err != nil {
		finishLoginError(attempt, ctx, "setup_failed", err)
		return
	}
	attempt.setInstance(inst)
	defer m.cleanupLoginInstance(inst)
	if err := m.configureAntigravityLoginInstance(ctx, inst, ia, attempt); err != nil {
		finishLoginError(attempt, ctx, "setup_failed", err)
		return
	}
	attempt.update("waiting_for_browser", "", "", "")
	if err := m.authenticateAntigravityLogin(ctx, inst, attempt); err != nil {
		if ctx.Err() != nil {
			finishLoginCancellation(attempt, ctx)
			return
		}
		attempt.finish("failed", "auth_failed", sanitizeLoginError(err))
		return
	}
	if err := ctx.Err(); err != nil {
		finishLoginCancellation(attempt, ctx)
		return
	}
	attempt.update("checking", "", "", "")
	caps, err := m.Refresh(ctx, agents.AntigravityACPAgentID)
	if err != nil || caps.Status != StatusOK {
		if ctx.Err() != nil {
			finishLoginCancellation(attempt, ctx)
			return
		}
		attempt.finish("succeeded", "catalog_unavailable", "Signed in, but model discovery failed. Retry model discovery.")
		return
	}
	attempt.finish("succeeded", "", "")
}

func (a *antigravityLoginAttempt) setInstance(inst *instance) {
	a.mu.Lock()
	a.inst = inst
	a.mu.Unlock()
}

func (m *Manager) createAntigravityLoginInstance(
	ctx context.Context,
	attempt *antigravityLoginAttempt,
) (*instance, agents.InferenceAgent, error) {
	m.mu.RLock()
	parentTmpDir := m.parentTmpDir
	m.mu.RUnlock()
	ia, ok := m.registry.GetInferenceAgent(agents.AntigravityACPAgentID)
	if !ok {
		return nil, nil, errors.New("antigravity agent is not inference-capable")
	}
	workDir := fmt.Sprintf("%s/antigravity-login-%s", parentTmpDir, attempt.id)
	req := &agentctlCreateInstanceRequest{
		Protocol:   "acp",
		AgentType:  agents.AntigravityACPAgentID,
		Env:        agents.RuntimeEnvFor(ia),
		StripEnv:   agents.StripEnvFor(ia),
		AuthPolicy: acpcompat.AntigravityAuthPolicy(true),
	}
	req.AuthPolicy.AttemptID = attempt.id
	inst, err := m.createInstanceWithRequest(ctx, agents.AntigravityACPAgentID, workDir, req)
	if err != nil {
		return nil, nil, err
	}
	return inst, ia, nil
}

func (m *Manager) configureAntigravityLoginInstance(
	ctx context.Context,
	inst *instance,
	ia agents.InferenceAgent,
	attempt *antigravityLoginAttempt,
) error {
	command, localEnv, err := m.resolveInferenceCommand(ctx, agents.AntigravityACPAgentID, ia, agents.Command{})
	if err != nil {
		return err
	}
	args := command.Args()
	if len(args) == 0 {
		return errors.New("antigravity runtime command is empty")
	}
	if err := inst.client.RequireCapability(ctx, acpcompat.AntigravityPersonalAuthCapability); err != nil {
		return err
	}
	env := mergeInferenceEnv(agents.RuntimeEnvFor(ia), localEnv)
	if err := inst.client.ConfigureAgent(ctx, args[0], args, env, "", nil); err != nil {
		return fmt.Errorf("configure antigravity login instance: %w", err)
	}
	if _, err := inst.client.Start(ctx); err != nil {
		return fmt.Errorf("start antigravity login instance: %w", err)
	}
	streamErr := make(chan error, 1)
	go func() {
		streamErr <- inst.client.StreamUpdates(ctx, func(agentctlAgentEvent) {}, nil, nil)
	}()
	if err := waitForAntigravityStream(ctx, inst.client, streamErr); err != nil {
		return err
	}
	if _, err := inst.client.Initialize(ctx, "kandev", "1.0.0"); err != nil {
		return fmt.Errorf("initialize antigravity login instance: %w", err)
	}
	return nil
}

func waitForAntigravityStream(
	ctx context.Context,
	client *agentctlClient,
	streamErr <-chan error,
) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		if client.HasAgentStream() {
			return nil
		}
		select {
		case err := <-streamErr:
			if err != nil {
				return fmt.Errorf("open antigravity login stream: %w", err)
			}
			return errors.New("antigravity login stream closed before it connected")
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return errors.New("antigravity login stream did not connect")
		case <-ticker.C:
		}
	}
}

func (m *Manager) authenticateAntigravityLogin(
	ctx context.Context,
	inst *instance,
	attempt *antigravityLoginAttempt,
) error {
	done := make(chan error, 1)
	go func() {
		done <- inst.client.Authenticate(ctx, acpcompat.AntigravityPersonalAuthMethod)
	}()
	ticker := time.NewTicker(antigravityLoginPoll)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			status, err := inst.client.AuthStatus(ctx, attempt.id)
			if err == nil && status.AuthorizationURL != "" {
				attempt.update("waiting_for_browser", status.AuthorizationURL, "", "")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *Manager) cleanupLoginInstance(inst *instance) {
	if inst == nil {
		return
	}
	ctx, cancel := hostUtilityDeleteContext(context.Background())
	defer cancel()
	m.deleteInstance(ctx, inst)
}

func finishLoginError(attempt *antigravityLoginAttempt, ctx context.Context, code string, err error) {
	if ctx.Err() != nil {
		finishLoginCancellation(attempt, ctx)
		return
	}
	attempt.finish("failed", code, sanitizeLoginError(err))
}

func finishLoginCancellation(attempt *antigravityLoginAttempt, ctx context.Context) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		attempt.finish("expired", "expired", "Antigravity sign-in expired")
		return
	}
	attempt.finish("cancelled", "cancelled", "Antigravity sign-in was cancelled")
}

func sanitizeLoginError(err error) string {
	if err == nil {
		return ""
	}
	message := streams.SanitizeProviderMessage(acpcompat.RedactConsentURLs(err.Error()))
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
