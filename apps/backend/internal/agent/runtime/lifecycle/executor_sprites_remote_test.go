package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sprites "github.com/superfly/sprites-go"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	spritesutil "github.com/kandev/kandev/internal/sprites"
)

// spritesAPIServer stands in for the Sprites control-plane REST API. Only the
// plain-HTTP endpoints the executor uses are served; the WebSocket exec channel
// is deliberately out of scope (see the package test notes).
type spritesAPIServer struct {
	mu           sync.Mutex
	created      []string
	getStatus    int
	getBody      string
	createStatus int
	server       *httptest.Server
}

func newSpritesAPIServer(t *testing.T) *spritesAPIServer {
	t.Helper()
	s := &spritesAPIServer{
		getStatus:    http.StatusOK,
		createStatus: http.StatusCreated,
		getBody:      `{"name":"kandev-abc","id":"sprite-1","status":"running"}`,
	}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sprites":
			var body struct {
				Name string `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.created = append(s.created, body.Name)
			w.WriteHeader(s.createStatus)
			_, _ = w.Write([]byte(`{"name":"` + body.Name + `"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sprites/"):
			w.WriteHeader(s.getStatus)
			if s.getStatus == http.StatusOK {
				_, _ = w.Write([]byte(s.getBody))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *spritesAPIServer) client() *sprites.Client {
	return sprites.NewClient(s.server.URL, "test-token")
}

func TestSpritesExecutorStaticSurface(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	if exec.Name() != executor.NameSprites {
		t.Fatalf("Name() = %q", exec.Name())
	}
	if err := exec.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck = %v, want nil", err)
	}
	if !exec.RequiresCloneURL() {
		t.Fatal("sprites clone the repository inside the sandbox")
	}
	if exec.ShouldApplyPreferredShell() {
		t.Fatal("the host shell must not be applied to a cloud sandbox")
	}
	if !exec.IsAlwaysResumable() {
		t.Fatal("sprite sandboxes are always resumable")
	}
	if exec.GetInteractiveRunner() != nil {
		t.Fatal("sprites have no host-side interactive runner")
	}
	instances, err := exec.RecoverInstances(context.Background())
	if err != nil || instances != nil {
		t.Fatalf("RecoverInstances() = %v, %v", instances, err)
	}
	if err := exec.ResumeRemoteInstance(context.Background(), &ExecutorCreateRequest{}); err != nil {
		t.Fatalf("ResumeRemoteInstance = %v, want nil", err)
	}
	if err := exec.ResumeRemoteInstance(context.Background(), nil); err == nil {
		t.Fatal("a nil request must be rejected")
	}
}

func TestSpritesExecutorCreateSprite(t *testing.T) {
	api := newSpritesAPIServer(t)
	exec := newTestSpritesExecutor(nil)

	sprite, err := exec.createSprite(context.Background(), api.client(), "kandev-abc123456789")
	if err != nil {
		t.Fatalf("createSprite: %v", err)
	}
	if sprite == nil {
		t.Fatal("createSprite returned no sprite")
	}
	api.mu.Lock()
	created := append([]string(nil), api.created...)
	api.mu.Unlock()
	if len(created) != 1 || created[0] != "kandev-abc123456789" {
		t.Fatalf("created sprites = %v", created)
	}

	t.Run("API failure is wrapped", func(t *testing.T) {
		api.mu.Lock()
		api.createStatus = http.StatusInternalServerError
		api.mu.Unlock()
		_, err := exec.createSprite(context.Background(), api.client(), "kandev-fail")
		if err == nil || !strings.Contains(err.Error(), "failed to create sprite") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSpritesExecutorReconnectSprite(t *testing.T) {
	api := newSpritesAPIServer(t)
	exec := newTestSpritesExecutor(nil)

	sprite, err := exec.reconnectSprite(context.Background(), api.client(), "kandev-abc")
	if err != nil {
		t.Fatalf("reconnectSprite: %v", err)
	}
	if sprite.Status != "running" {
		t.Fatalf("sprite status = %q", sprite.Status)
	}

	t.Run("a missing sandbox is classified as not-found", func(t *testing.T) {
		api.mu.Lock()
		api.getStatus = http.StatusNotFound
		api.mu.Unlock()
		_, err := exec.reconnectSprite(context.Background(), api.client(), "kandev-gone")
		if !errors.Is(err, spritesutil.ErrSpriteNotFound) {
			t.Fatalf("error = %v, want ErrSpriteNotFound so CreateInstance can reprovision", err)
		}
	})
}

func TestSpritesExecutorStepCreateSpriteReportsProgress(t *testing.T) {
	api := newSpritesAPIServer(t)
	exec := newTestSpritesExecutor(nil)

	t.Run("fresh provisioning", func(t *testing.T) {
		var steps []PrepareStep
		report := newSpritesStepReporter(func(step PrepareStep, _, _ int) {
			steps = append(steps, step)
		}, newSpritesProgressPlan(false))

		_, err := exec.stepCreateSprite(context.Background(), api.client(), "kandev-abc", false, report)
		if err != nil {
			t.Fatalf("stepCreateSprite: %v", err)
		}
		if len(steps) != 2 || steps[0].Name != "Creating cloud sandbox" {
			t.Fatalf("steps = %+v", steps)
		}
		if steps[1].Status != PrepareStepCompleted {
			t.Fatalf("final step = %+v", steps[1])
		}
	})

	t.Run("reconnect names the step differently", func(t *testing.T) {
		var steps []PrepareStep
		report := newSpritesStepReporter(func(step PrepareStep, _, _ int) {
			steps = append(steps, step)
		}, newSpritesProgressPlan(true))

		_, err := exec.stepCreateSprite(context.Background(), api.client(), "kandev-abc", true, report)
		if err != nil {
			t.Fatalf("stepCreateSprite: %v", err)
		}
		if steps[0].Name != "Reconnecting cloud sandbox" {
			t.Fatalf("step name = %q", steps[0].Name)
		}
	})

	t.Run("failure is reported on the step", func(t *testing.T) {
		api.mu.Lock()
		api.getStatus = http.StatusNotFound
		api.mu.Unlock()
		var steps []PrepareStep
		report := newSpritesStepReporter(func(step PrepareStep, _, _ int) {
			steps = append(steps, step)
		}, newSpritesProgressPlan(true))

		_, err := exec.stepCreateSprite(context.Background(), api.client(), "kandev-gone", true, report)
		if err == nil {
			t.Fatal("expected the reconnect to fail")
		}
		last := steps[len(steps)-1]
		if last.Status != PrepareStepFailed || last.Error == "" {
			t.Fatalf("final step = %+v, want a failed step carrying the error", last)
		}
	})
}

func TestSpritesResolveSpriteName(t *testing.T) {
	exec := newTestSpritesExecutor(nil)

	t.Run("fresh launches derive the name from the instance ID", func(t *testing.T) {
		got := exec.resolveSpriteName(&ExecutorCreateRequest{InstanceID: "0123456789abcdef"}, false)
		if got != "kandev-0123456789ab" {
			t.Fatalf("sprite name = %q, want the 12-char instance prefix", got)
		}
	})

	t.Run("reconnect prefers the recorded sprite name", func(t *testing.T) {
		got := exec.resolveSpriteName(&ExecutorCreateRequest{
			InstanceID: "0123456789abcdef",
			Metadata:   map[string]interface{}{MetadataKeySpriteName: "kandev-existing"},
		}, true)
		if got != "kandev-existing" {
			t.Fatalf("sprite name = %q", got)
		}
	})

	t.Run("reconnect without a recorded name falls back to the previous execution", func(t *testing.T) {
		got := exec.resolveSpriteName(&ExecutorCreateRequest{
			InstanceID:          "0123456789abcdef",
			PreviousExecutionID: "fedcba9876543210",
		}, true)
		if got != "kandev-fedcba987654" {
			t.Fatalf("sprite name = %q", got)
		}
		short := exec.resolveSpriteName(&ExecutorCreateRequest{PreviousExecutionID: "short"}, true)
		if short != "kandev-short" {
			t.Fatalf("sprite name = %q", short)
		}
	})
}

func TestSpritesShouldReconnect(t *testing.T) {
	if spritesShouldReconnect(nil) {
		t.Fatal("a nil request must not reconnect")
	}
	if spritesShouldReconnect(&ExecutorCreateRequest{}) {
		t.Fatal("a fresh request must not reconnect")
	}
	if !spritesShouldReconnect(&ExecutorCreateRequest{PreviousExecutionID: "prev"}) {
		t.Fatal("a previous execution implies reconnect")
	}
	if !spritesShouldReconnect(&ExecutorCreateRequest{
		Metadata: map[string]interface{}{MetadataKeySpriteName: "kandev-x"},
	}) {
		t.Fatal("a recorded sprite name implies reconnect")
	}
}

func TestSpritesAgentInstanceID(t *testing.T) {
	if got := spritesAgentInstanceID(nil); got != "" {
		t.Fatalf("nil request = %q", got)
	}
	if got := spritesAgentInstanceID(&ExecutorCreateRequest{InstanceID: "cur", PreviousExecutionID: "prev"}); got != "cur" {
		t.Fatalf("instance id = %q, want the current instance", got)
	}
	if got := spritesAgentInstanceID(&ExecutorCreateRequest{PreviousExecutionID: "prev"}); got != "prev" {
		t.Fatalf("instance id = %q", got)
	}
}

func TestSpritesFallbackToFreshSandboxRewritesThePlan(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	plan := newSpritesProgressPlan(true)
	var steps []PrepareStep
	var totals []int
	report := newSpritesStepReporter(func(step PrepareStep, _, total int) {
		steps = append(steps, step)
		totals = append(totals, total)
	}, plan)

	newName := exec.fallbackToFreshSandbox(&ExecutorCreateRequest{
		InstanceID: "0123456789abcdef",
		Metadata:   map[string]interface{}{MetadataKeyWorktreeBranch: "feature/task-1"},
	}, plan, report, "kandev-old")

	if newName != "kandev-0123456789ab" {
		t.Fatalf("new sprite name = %q", newName)
	}
	if plan.total() != 7 {
		t.Fatalf("plan total = %d, want the full fresh-provision plan", plan.total())
	}
	if !plan.has(spriteStepUploadAgentctl) || !plan.has(spriteStepApplyNetworkPolicy) {
		t.Fatal("the fresh plan must include the setup and network-policy steps")
	}
	if len(steps) != 1 {
		t.Fatalf("steps = %+v, want one explanatory notice", steps)
	}
	notice := steps[0]
	if notice.Name != "Reconnecting cloud sandbox" || notice.Status != PrepareStepSkipped {
		t.Fatalf("notice = %+v", notice)
	}
	if notice.Warning == "" || !strings.Contains(notice.WarningDetail, "kandev-old") ||
		!strings.Contains(notice.WarningDetail, newName) {
		t.Fatalf("notice must explain the swap: %+v", notice)
	}
	if !strings.Contains(notice.WarningDetail, "feature/task-1") {
		t.Fatalf("notice must name the branch: %q", notice.WarningDetail)
	}
	if totals[0] != 7 {
		t.Fatalf("reported total = %d, want the replaced plan's total", totals[0])
	}
}

func TestSpritesFallbackToFreshSandboxUsesBaseBranchAndPlaceholder(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	plan := newSpritesProgressPlan(true)
	var steps []PrepareStep
	report := newSpritesStepReporter(func(step PrepareStep, _, _ int) { steps = append(steps, step) }, plan)

	exec.fallbackToFreshSandbox(&ExecutorCreateRequest{
		InstanceID: "0123456789abcdef",
		Metadata:   map[string]interface{}{MetadataKeyBaseBranch: "main"},
	}, plan, report, "kandev-old")
	if !strings.Contains(steps[0].WarningDetail, "main") {
		t.Fatalf("detail = %q, want the base branch when no worktree branch is set", steps[0].WarningDetail)
	}

	if got := branchOrPlaceholder(""); got != "(unknown)" {
		t.Fatalf("branchOrPlaceholder(\"\") = %q", got)
	}
	if got := branchOrPlaceholder("dev"); got != "dev" {
		t.Fatalf("branchOrPlaceholder(\"dev\") = %q", got)
	}
}

func TestSpritesBuildInstanceResult(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	createdAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	sprite := &sprites.Sprite{Status: "  running  ", CreatedAt: createdAt}

	instance := exec.buildInstanceResult(&ExecutorCreateRequest{
		InstanceID: "instance-1", TaskID: "task-1", SessionID: "session-1",
	}, "kandev-abc", sprite, 51234, 8765, true)

	if instance.InstanceID != "instance-1" || instance.TaskID != "task-1" || instance.SessionID != "session-1" {
		t.Fatalf("identity = %+v", instance)
	}
	if instance.RuntimeName != executor.NameSprites {
		t.Fatalf("RuntimeName = %q", instance.RuntimeName)
	}
	if instance.WorkspacePath != spritesWorkspacePath {
		t.Fatalf("WorkspacePath = %q", instance.WorkspacePath)
	}
	if instance.AuthToken != "" {
		t.Fatalf("AuthToken = %q; sprites are isolated VMs and use no agentctl token", instance.AuthToken)
	}
	if instance.Metadata[MetadataKeySpriteName] != "kandev-abc" {
		t.Fatalf("sprite name metadata = %v", instance.Metadata[MetadataKeySpriteName])
	}
	if instance.Metadata[MetadataKeySpriteState] != "running" {
		t.Fatalf("sprite state metadata = %v, want it trimmed", instance.Metadata[MetadataKeySpriteState])
	}
	if instance.Metadata[MetadataKeySpriteCreatedAt] != createdAt {
		t.Fatalf("created-at metadata = %v", instance.Metadata[MetadataKeySpriteCreatedAt])
	}
	if instance.Metadata[MetadataKeyLocalPort] != 51234 {
		t.Fatalf("local port metadata = %v", instance.Metadata[MetadataKeyLocalPort])
	}
	if instance.Metadata["reuse_existing_process"] != true || instance.Metadata[MetadataKeyIsRemote] != true {
		t.Fatalf("metadata = %+v", instance.Metadata)
	}
	if instance.Client == nil {
		t.Fatal("instance must carry an agentctl client on the forwarded local port")
	}
}

func TestSpriteCreateInstanceRequestMapsEveryField(t *testing.T) {
	autoApprove := false
	req := &ExecutorCreateRequest{
		InstanceID:                     "instance-1",
		TaskID:                         "task-1",
		SessionID:                      "session-1",
		Protocol:                       "acp",
		McpMode:                        "task",
		McpProviders:                   []string{"github"},
		AutoApprovePermissions:         true,
		AutoApprovePermissionsOverride: &autoApprove,
		AgentConfig: &fakeRuntimeAgent{
			MockAgent: agents.NewMockAgent(), id: "codex",
			requiresKill: true, stripEnv: []string{"NODE_OPTIONS"},
		},
		Env:      map[string]string{"OPENAI_API_KEY": "sk-1"},
		Metadata: map[string]interface{}{MetadataKeyBaseBranches: map[string]string{"": "main"}},
	}

	got := spriteCreateInstanceRequest(req)

	if got.ID != "instance-1" || got.WorkspacePath != spritesWorkspacePath {
		t.Fatalf("request = %+v", got)
	}
	if got.AgentType != "codex" || !got.RequiresProcessKill {
		t.Fatalf("agent fields = %+v", got)
	}
	if !equalStrings(got.StripEnv, []string{"NODE_OPTIONS"}) {
		t.Fatalf("StripEnv = %v", got.StripEnv)
	}
	if got.AutoApprovePermissions == nil || *got.AutoApprovePermissions {
		t.Fatalf("AutoApprovePermissions = %v, want the explicit profile override to win", got.AutoApprovePermissions)
	}
	if got.BaseBranches[""] != "main" {
		t.Fatalf("BaseBranches = %+v", got.BaseBranches)
	}
	// Unlike SSH, the sandbox is isolated so the full env is forwarded.
	if got.Env["OPENAI_API_KEY"] != "sk-1" {
		t.Fatalf("Env = %+v", got.Env)
	}
	req.Env["MUTATED"] = "after"
	if _, leaked := got.Env["MUTATED"]; leaked {
		t.Fatalf("the request env must be cloned, not aliased: %+v", got.Env)
	}
}

func TestAgentAndRuntimeFieldsFromRequest(t *testing.T) {
	if got := agentTypeFromReq(&ExecutorCreateRequest{}); got != "" {
		t.Fatalf("agent type = %q", got)
	}
	if got := agentTypeFromReq(&ExecutorCreateRequest{AgentConfig: newFakeIdentityAgent("codex")}); got != "codex" {
		t.Fatalf("agent type = %q", got)
	}

	if requiresProcessKillFromReq(nil) || requiresProcessKillFromReq(&ExecutorCreateRequest{}) {
		t.Fatal("missing agent config must not require a process kill")
	}
	if stripEnvFromReq(nil) != nil || stripEnvFromReq(&ExecutorCreateRequest{}) != nil {
		t.Fatal("missing agent config must yield no strip list")
	}
	req := &ExecutorCreateRequest{AgentConfig: &fakeRuntimeAgent{
		MockAgent: agents.NewMockAgent(), id: "opencode",
		requiresKill: true, stripEnv: []string{"NODE_OPTIONS"},
	}}
	if !requiresProcessKillFromReq(req) {
		t.Fatal("the agent runtime config must be honoured")
	}
	if !equalStrings(stripEnvFromReq(req), []string{"NODE_OPTIONS"}) {
		t.Fatalf("strip env = %v", stripEnvFromReq(req))
	}
}

// fakeReconnectControl records the reconnect delete/create sequence.
type fakeReconnectControl struct {
	deleted   []string
	created   int
	deleteErr error
	createErr error
	port      int
}

func (c *fakeReconnectControl) Delete(_ context.Context, instanceID string) error {
	c.deleted = append(c.deleted, instanceID)
	return c.deleteErr
}

func (c *fakeReconnectControl) Create(context.Context, *ExecutorCreateRequest) (int, error) {
	c.created++
	return c.port, c.createErr
}

func TestReplaceSpriteReconnectInstance(t *testing.T) {
	t.Run("deletes before creating", func(t *testing.T) {
		control := &fakeReconnectControl{port: 8123}
		port, err := replaceSpriteReconnectInstance(context.Background(), control,
			&ExecutorCreateRequest{InstanceID: "instance-1"}, "instance-1")
		if err != nil {
			t.Fatalf("replaceSpriteReconnectInstance: %v", err)
		}
		if port != 8123 {
			t.Fatalf("port = %d", port)
		}
		if len(control.deleted) != 1 || control.deleted[0] != "instance-1" || control.created != 1 {
			t.Fatalf("control = %+v", control)
		}
	})

	t.Run("a failed delete skips the create", func(t *testing.T) {
		wantErr := errors.New("delete failed")
		control := &fakeReconnectControl{deleteErr: wantErr}
		_, err := replaceSpriteReconnectInstance(context.Background(), control,
			&ExecutorCreateRequest{}, "instance-1")
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
		if control.created != 0 {
			t.Fatal("a failed delete must not be followed by a create")
		}
	})

	t.Run("a failed create bubbles up", func(t *testing.T) {
		wantErr := errors.New("create failed")
		control := &fakeReconnectControl{createErr: wantErr}
		_, err := replaceSpriteReconnectInstance(context.Background(), control,
			&ExecutorCreateRequest{}, "instance-1")
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSpritesExecutorStopInstanceEarlyReturns(t *testing.T) {
	t.Run("nil instance", func(t *testing.T) {
		if err := newTestSpritesExecutor(nil).StopInstance(context.Background(), nil, false); err != nil {
			t.Fatalf("StopInstance(nil): %v", err)
		}
	})

	t.Run("no sprite name recorded", func(t *testing.T) {
		exec := newTestSpritesExecutor(nil)
		if err := exec.StopInstance(context.Background(), &ExecutorInstance{InstanceID: "i"}, false); err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
	})

	t.Run("a non-terminal stop preserves the sandbox and drops the proxy", func(t *testing.T) {
		exec := newTestSpritesExecutor(nil)
		cancelled := false
		exec.proxies["i"] = &SpritesProxySession{
			spriteName: "kandev-abc",
			cancel:     func() { cancelled = true },
		}
		exec.tokens["i"] = "token"

		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "i",
			StopReason: "stopped via API",
			Metadata:   map[string]interface{}{MetadataKeySpriteName: "kandev-abc"},
		}, false)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		if !cancelled {
			t.Fatal("the local proxy session must be torn down on every stop")
		}
		if _, tracked := exec.proxies["i"]; tracked {
			t.Fatal("the proxy must be dropped from the map")
		}
		if exec.tokens["i"] != "token" {
			t.Fatal("a preserved sandbox must keep its cached token for resume")
		}
	})

	t.Run("a terminal stop without a token cannot destroy and says so", func(t *testing.T) {
		exec := newTestSpritesExecutor(nil)
		err := exec.StopInstance(context.Background(), &ExecutorInstance{
			InstanceID: "i",
			StopReason: StopReasonTaskDeleted,
			Metadata:   map[string]interface{}{MetadataKeySpriteName: "kandev-abc"},
		}, false)
		if err != nil {
			t.Fatalf("StopInstance without a token must not error: %v", err)
		}
	})
}

func TestSpritesExecutorGetRemoteStatusWithoutAToken(t *testing.T) {
	exec := newTestSpritesExecutor(nil)

	t.Run("nil instance", func(t *testing.T) {
		if _, err := exec.GetRemoteStatus(context.Background(), nil); err == nil {
			t.Fatal("expected an error for a nil instance")
		}
	})

	t.Run("no sprite name yields unknown", func(t *testing.T) {
		status, err := exec.GetRemoteStatus(context.Background(), &ExecutorInstance{InstanceID: "i"})
		if err != nil {
			t.Fatalf("GetRemoteStatus: %v", err)
		}
		if status.State != "unknown" || status.RuntimeName != executor.NameSprites {
			t.Fatalf("status = %+v", status)
		}
		if status.LastCheckedAt.IsZero() {
			t.Fatal("LastCheckedAt must be stamped")
		}
	})

	t.Run("no resolvable token yields unknown but names the sandbox", func(t *testing.T) {
		status, err := exec.GetRemoteStatus(context.Background(), &ExecutorInstance{
			InstanceID: "i",
			Metadata:   map[string]interface{}{MetadataKeySpriteName: "  kandev-abc  "},
		})
		if err != nil {
			t.Fatalf("GetRemoteStatus: %v", err)
		}
		if status.State != "unknown" || status.RemoteName != "kandev-abc" {
			t.Fatalf("status = %+v", status)
		}
	})
}

func TestSpritesExecutorCleanupOnFailure(t *testing.T) {
	t.Run("nil sprite is a no-op", func(t *testing.T) {
		newTestSpritesExecutor(nil).cleanupOnFailure(context.Background(), nil, "i", true)
	})

	t.Run("reconnect flow preserves the sandbox but drops local state", func(t *testing.T) {
		exec := newTestSpritesExecutor(nil)
		cancelled := false
		exec.proxies["i"] = &SpritesProxySession{cancel: func() { cancelled = true }}
		exec.tokens["i"] = "token"

		exec.cleanupOnFailure(context.Background(), &sprites.Sprite{}, "i", false)

		if !cancelled {
			t.Fatal("the proxy session must be cancelled")
		}
		if _, tracked := exec.proxies["i"]; tracked {
			t.Fatal("the proxy must be dropped")
		}
		if _, tracked := exec.tokens["i"]; tracked {
			t.Fatal("the cached token must be dropped")
		}
	})
}

func TestSpritesCloseProxySessionIsNilSafe(t *testing.T) {
	exec := newTestSpritesExecutor(nil)
	exec.closeProxySession(nil)
	exec.closeProxySession(&SpritesProxySession{})
	cancelled := false
	exec.closeProxySession(&SpritesProxySession{cancel: func() { cancelled = true }})
	if !cancelled {
		t.Fatal("cancel must be invoked when present")
	}
}

func TestNonZeroTimePtr(t *testing.T) {
	if nonZeroTimePtr(time.Time{}) != nil {
		t.Fatal("a zero time must map to nil")
	}
	stamp := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	got := nonZeroTimePtr(stamp)
	if got == nil || !got.Equal(stamp) {
		t.Fatalf("nonZeroTimePtr = %v", got)
	}
}

func TestGetFreePortReturnsABindablePort(t *testing.T) {
	port, err := getFreePort()
	if err != nil {
		t.Fatalf("getFreePort: %v", err)
	}
	if port < 1 || port > 65535 {
		t.Fatalf("port = %d, out of range", port)
	}
}

func TestSpritesApplyNetworkPolicy(t *testing.T) {
	exec := newTestSpritesExecutor(nil)

	t.Run("no rules configured is a no-op", func(t *testing.T) {
		api := newSpritesAPIServer(t)
		if err := exec.applyNetworkPolicy(context.Background(), api.client(), "kandev-abc",
			&ExecutorCreateRequest{Metadata: map[string]interface{}{}}); err != nil {
			t.Fatalf("applyNetworkPolicy: %v", err)
		}
	})

	t.Run("an empty rule list is a no-op", func(t *testing.T) {
		api := newSpritesAPIServer(t)
		if err := exec.applyNetworkPolicy(context.Background(), api.client(), "kandev-abc",
			&ExecutorCreateRequest{Metadata: map[string]interface{}{
				"sprites_network_policy_rules": `[]`,
			}}); err != nil {
			t.Fatalf("applyNetworkPolicy: %v", err)
		}
	})

	t.Run("malformed rules are rejected", func(t *testing.T) {
		api := newSpritesAPIServer(t)
		err := exec.applyNetworkPolicy(context.Background(), api.client(), "kandev-abc",
			&ExecutorCreateRequest{Metadata: map[string]interface{}{
				"sprites_network_policy_rules": `{not json`,
			}})
		if err == nil || !strings.Contains(err.Error(), "failed to parse network policy rules") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSpritesResolvePrepareScript(t *testing.T) {
	claude := &fakeIdentityAgent{
		MockAgent: agents.NewMockAgent(), id: "claude-code", installScript: "npm i -g claude",
	}
	exec := NewSpritesExecutor(nil, &mockAgentLister{agents: []agents.Agent{claude}},
		nil, spritesAgentctlPort, newTestLogger())

	t.Run("an explicit setup script gets the branch-checkout postlude", func(t *testing.T) {
		script, err := exec.resolvePrepareScript(&ExecutorCreateRequest{
			TaskID: "task-1",
			Metadata: map[string]interface{}{
				MetadataKeySetupScript:    "printf setup > {{workspace.path}}/marker",
				MetadataKeyBaseBranch:     "main",
				MetadataKeyWorktreeBranch: "feature/task-1",
			},
		})
		if err != nil {
			t.Fatalf("resolvePrepareScript: %v", err)
		}
		if !strings.Contains(script, "printf setup > '/workspace'/marker") {
			t.Fatalf("workspace placeholder unresolved:\n%s", script)
		}
		if !strings.Contains(script, "kandev-managed: ensure session feature branch is checked out") {
			t.Fatalf("branch-checkout postlude missing:\n%s", script)
		}
		if !strings.Contains(script, "feature/task-1") {
			t.Fatalf("session branch missing:\n%s", script)
		}
	})

	t.Run("selected agents contribute their install scripts", func(t *testing.T) {
		script, err := exec.resolvePrepareScript(&ExecutorCreateRequest{
			TaskID:      "task-1",
			AgentConfig: claude,
			Metadata: map[string]interface{}{
				MetadataKeySetupScript: "{{kandev.agents.install}}",
				MetadataKeyBaseBranch:  "main",
			},
		})
		if err != nil {
			t.Fatalf("resolvePrepareScript: %v", err)
		}
		if !strings.Contains(script, "npm i -g claude") {
			t.Fatalf("agent install script missing:\n%s", script)
		}
	})
}
