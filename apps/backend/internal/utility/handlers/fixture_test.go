package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/utility/controller"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/service"
)

// fakeUtilityRepo is an in-memory store.Repository. Missing rows are reported
// as sql.ErrNoRows because that is the shape the service maps to
// ErrAgentNotFound; anything else falls through as a 500.
type fakeUtilityRepo struct {
	mu         sync.Mutex
	agents     map[string]*models.UtilityAgent
	agentOrder []string
	calls      map[string]*models.UtilityAgentCall
	callOrder  []string

	// lastListCallsLimit records the limit the handler resolved, so the
	// clamping rules are observable from a test.
	lastListCallsLimit int
	errs               map[string]error
	seq                int
	createCallCount    int
	updateCallCount    int
	cancelAfterCreate  context.CancelFunc
}

func newFakeUtilityRepo() *fakeUtilityRepo {
	return &fakeUtilityRepo{
		agents: map[string]*models.UtilityAgent{},
		calls:  map[string]*models.UtilityAgentCall{},
		errs:   map[string]error{},
	}
}

// failWith makes the named repository method return err. Guarded by the same
// mutex as every other field: the concurrent-refresh test drives 50 requests at
// once, so a future test that injects a failure mid-flight must not race.
func (r *fakeUtilityRepo) failWith(method string, err error) *fakeUtilityRepo {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs[method] = err
	return r
}

// fail returns the injected error for a method, if any. Callers must not hold
// r.mu.
func (r *fakeUtilityRepo) fail(method string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.errs[method]
}

// putAgent seeds an agent row, preserving insertion order for ListAgents.
func (r *fakeUtilityRepo) putAgent(agent *models.UtilityAgent) *models.UtilityAgent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[agent.ID]; !exists {
		r.agentOrder = append(r.agentOrder, agent.ID)
	}
	r.agents[agent.ID] = agent
	return agent
}

// call returns a stored call record.
func (r *fakeUtilityRepo) call(id string) *models.UtilityAgentCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneUtilityCall(r.calls[id])
}

func cloneUtilityCall(call *models.UtilityAgentCall) *models.UtilityAgentCall {
	if call == nil {
		return nil
	}
	clone := *call
	if call.CompletedAt != nil {
		completedAt := *call.CompletedAt
		clone.CompletedAt = &completedAt
	}
	return &clone
}

// listCallsLimit returns the limit the handler last resolved.
func (r *fakeUtilityRepo) listCallsLimit() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastListCallsLimit
}

// callCount returns how many call records were created.
func (r *fakeUtilityRepo) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *fakeUtilityRepo) callWriteCounts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.createCallCount, r.updateCallCount
}

func (r *fakeUtilityRepo) cancelAfterNextCallCreate(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelAfterCreate = cancel
}

func (r *fakeUtilityRepo) ListAgents(context.Context) ([]*models.UtilityAgent, error) {
	if err := r.fail("ListAgents"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*models.UtilityAgent, 0, len(r.agentOrder))
	for _, id := range r.agentOrder {
		if agent, ok := r.agents[id]; ok {
			out = append(out, agent)
		}
	}
	return out, nil
}

func (r *fakeUtilityRepo) GetAgentByID(_ context.Context, id string) (*models.UtilityAgent, error) {
	if err := r.fail("GetAgentByID"); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if agent, ok := r.agents[id]; ok {
		return agent, nil
	}
	return nil, sql.ErrNoRows
}

func (r *fakeUtilityRepo) GetAgentByName(_ context.Context, name string) (*models.UtilityAgent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, agent := range r.agents {
		if agent.Name == name {
			return agent, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeUtilityRepo) CreateAgent(_ context.Context, agent *models.UtilityAgent) error {
	if err := r.fail("CreateAgent"); err != nil {
		return err
	}
	r.mu.Lock()
	r.seq++
	if agent.ID == "" {
		agent.ID = fmt.Sprintf("utility-%d", r.seq)
	}
	now := time.Now().UTC()
	agent.CreatedAt, agent.UpdatedAt = now, now
	r.mu.Unlock()
	r.putAgent(agent)
	return nil
}

func (r *fakeUtilityRepo) UpdateAgent(_ context.Context, agent *models.UtilityAgent) error {
	if err := r.fail("UpdateAgent"); err != nil {
		return err
	}
	agent.UpdatedAt = time.Now().UTC()
	r.putAgent(agent)
	return nil
}

func (r *fakeUtilityRepo) NormalizeEmptyBuiltinBinding(context.Context, string) (bool, error) {
	return false, nil
}

func (r *fakeUtilityRepo) DeleteAgent(_ context.Context, id string) error {
	if err := r.fail("DeleteAgent"); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.agents[id]; !ok {
		return sql.ErrNoRows
	}
	delete(r.agents, id)
	// Drop the ordering entry too. putAgent only appends when the ID is absent
	// from `agents`, so leaving a stale entry here would make a re-seeded ID
	// appear twice in ListAgents.
	for i, existing := range r.agentOrder {
		if existing == id {
			r.agentOrder = append(r.agentOrder[:i], r.agentOrder[i+1:]...)
			break
		}
	}
	return nil
}

func (r *fakeUtilityRepo) ListCalls(_ context.Context, utilityID string, limit int) ([]*models.UtilityAgentCall, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastListCallsLimit = limit
	if err := r.errs["ListCalls"]; err != nil {
		return nil, err
	}
	out := make([]*models.UtilityAgentCall, 0, len(r.callOrder))
	for _, id := range r.callOrder {
		if c, ok := r.calls[id]; ok && c.UtilityID == utilityID {
			out = append(out, cloneUtilityCall(c))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeUtilityRepo) GetCallByID(ctx context.Context, id string) (*models.UtilityAgentCall, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.calls[id]; ok {
		return cloneUtilityCall(c), nil
	}
	return nil, sql.ErrNoRows
}

func (r *fakeUtilityRepo) CreateCall(_ context.Context, call *models.UtilityAgentCall) error {
	if err := r.fail("CreateCall"); err != nil {
		return err
	}
	r.mu.Lock()
	r.seq++
	r.createCallCount++
	if call.ID == "" {
		call.ID = fmt.Sprintf("call-%d", r.seq)
	}
	r.calls[call.ID] = cloneUtilityCall(call)
	r.callOrder = append(r.callOrder, call.ID)
	cancel := r.cancelAfterCreate
	r.cancelAfterCreate = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (r *fakeUtilityRepo) UpdateCall(ctx context.Context, call *models.UtilityAgentCall) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateCallCount++
	r.calls[call.ID] = cloneUtilityCall(call)
	return nil
}

func TestFakeUtilityRepoCallWritesAreIsolated(t *testing.T) {
	repo := newFakeUtilityRepo()
	call := &models.UtilityAgentCall{ID: "call-1", Status: "pending"}
	if err := repo.CreateCall(context.Background(), call); err != nil {
		t.Fatalf("CreateCall: %v", err)
	}
	call.Status = "failed"
	if got := repo.call(call.ID); got == nil || got.Status != "pending" {
		t.Fatalf("mutating the create argument changed storage: %#v", got)
	}

	read, err := repo.GetCallByID(context.Background(), call.ID)
	if err != nil {
		t.Fatalf("GetCallByID: %v", err)
	}
	read.Status = "cancelled"
	if got := repo.call(call.ID); got == nil || got.Status != "pending" {
		t.Fatalf("mutating a read result changed storage: %#v", got)
	}

	update := cloneUtilityCall(read)
	update.Status = "completed"
	if err := repo.UpdateCall(context.Background(), update); err != nil {
		t.Fatalf("UpdateCall: %v", err)
	}
	update.Status = "failed"
	if got := repo.call(call.ID); got == nil || got.Status != "completed" {
		t.Fatalf("mutating the update argument changed storage: %#v", got)
	}
}

// stubUserSettings supplies the user's default utility agent/model pair.
type stubUserSettings struct {
	agentID string
	model   string
	err     error
}

func (s *stubUserSettings) GetDefaultUtilitySettings(context.Context) (string, string, error) {
	return s.agentID, s.model, s.err
}

// stubProfileResolver resolves an agent profile for the profile-bound
// execution path.
type stubProfileResolver struct {
	profile *agentsettingsmodels.AgentProfile
	err     error
}

func (r stubProfileResolver) Resolve(context.Context, string) (*agentsettingsmodels.AgentProfile, error) {
	return r.profile, r.err
}

func (r stubProfileResolver) MatchLegacy(
	context.Context, string, string,
) (*agentsettingsmodels.AgentProfile, error) {
	return r.profile, r.err
}

// utilityFixture is the full utility stack behind a live HTTP server: a fake
// store under the real service and controller, with every route from
// RegisterRoutes wired to stub executors.
type utilityFixture struct {
	router   *gin.Engine
	server   *httptest.Server
	repo     *fakeUtilityRepo
	service  *service.Service
	host     *statefulHostStub
	executor *stubInferenceExecutor
	settings *stubUserSettings
}

// utilityFixtureOptions tunes the pieces individual tests care about. The zero
// value gives a working stack with no agents registered and no host utility
// failures.
type utilityFixtureOptions struct {
	agents []lifecycle.InferenceAgentInfo
	host   *statefulHostStub
	// nilHost drops the host utility entirely, which is the "sessionless
	// execution unavailable" configuration.
	nilHost  bool
	settings *stubUserSettings
	// noSettings drops the user-settings provider so defaults never apply.
	noSettings bool
	executor   *stubInferenceExecutor
	resolver   service.ProfileResolver
}

func newUtilityFixture(t *testing.T, opts utilityFixtureOptions) *utilityFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}

	repo := newFakeUtilityRepo()
	svc := service.NewService(repo)
	if opts.resolver != nil {
		svc.SetProfileResolver(opts.resolver)
	}

	host := opts.host
	if host == nil && !opts.nilHost {
		host = newStatefulHostStub()
	}
	executor := opts.executor
	if executor == nil {
		executor = &stubInferenceExecutor{}
	}
	executor.agents = opts.agents
	settings := opts.settings
	if settings == nil && !opts.noSettings {
		settings = &stubUserSettings{}
	}

	router := gin.New()
	// Register through the production entry point so the route table under
	// test is the one the app serves.
	var hostExecutor HostUtilityExecutor
	if host != nil {
		hostExecutor = host
	}
	var userSettings UserSettingsProvider
	if settings != nil {
		userSettings = settings
	}
	RegisterRoutes(router, controller.NewController(svc), executor, hostExecutor, userSettings, log)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	return &utilityFixture{
		router:   router,
		server:   server,
		repo:     repo,
		service:  svc,
		host:     host,
		executor: executor,
		settings: settings,
	}
}
