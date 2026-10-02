package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/profiles"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

var errRead = errors.New("read failed")

type fakeContainment struct {
	execProfile *taskmodels.ExecutorProfile
	execErr     error
	executor    *taskmodels.Executor
	mode        auth.Mode
	agent       *settingsmodels.AgentProfile
	agentErr    error
	mcp         *settingsmodels.AgentProfileMcpConfig
	mcpErr      error
	secrets     map[string]fakeSecret
	creds       []CredentialValue
	credsErr    error
	bound       bool
	boundErr    error
	session     *ContainmentSession
	sessionErr  error
	reads       int
}

type fakeSecret struct {
	value     string
	updatedAt time.Time
	err       error
}

func (f *fakeContainment) GetExecutorProfile(context.Context, string) (*taskmodels.ExecutorProfile, error) {
	f.reads++
	return f.execProfile, f.execErr
}
func (f *fakeContainment) GetExecutor(context.Context, string) (*taskmodels.Executor, error) {
	return f.executor, nil
}
func (f *fakeContainment) Mode() auth.Mode { return f.mode }
func (f *fakeContainment) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return f.agent, f.agentErr
}
func (f *fakeContainment) GetAgentProfileMcpConfig(context.Context, string) (*settingsmodels.AgentProfileMcpConfig, error) {
	return f.mcp, f.mcpErr
}
func (f *fakeContainment) RevealSecret(_ context.Context, id string) (string, time.Time, error) {
	s := f.secrets[id]
	return s.value, s.updatedAt, s.err
}
func (f *fakeContainment) CredentialValues(context.Context, string) ([]CredentialValue, error) {
	return f.creds, f.credsErr
}
func (f *fakeContainment) HasRepositoryBinding(context.Context, string) (bool, error) {
	return f.bound, f.boundErr
}
func (f *fakeContainment) ConversationSession(context.Context, string) (*ContainmentSession, error) {
	return f.session, f.sessionErr
}

var (
	tLaunch = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	tBefore = tLaunch.Add(-time.Hour)
	tAfter  = tLaunch.Add(time.Hour)
)

func contained() *fakeContainment {
	return &fakeContainment{
		execProfile: &taskmodels.ExecutorProfile{ID: "ep", ExecutorID: "ex", UpdatedAt: tBefore},
		executor:    &taskmodels.Executor{Type: taskmodels.ExecutorTypeLocalDocker},
		mode:        auth.ModeEnabled,
		agent:       &settingsmodels.AgentProfile{ID: "ap", AgentID: "ag", UpdatedAt: tBefore},
		mcpErr:      sql.ErrNoRows,
		secrets:     map[string]fakeSecret{},
	}
}

func (f *fakeContainment) readers() ContainmentReaders {
	return ContainmentReaders{
		Executors: f, Auth: f, Agents: f, MCP: f, Secrets: f,
		Credentials: f, Repositories: f, Sessions: f,
	}
}

func conv(id string) *string { return &id }

func testCoordinator() *Coordinator {
	return &Coordinator{ID: "co-1", AgentProfileID: "ap", ExecutorProfileID: "ep", ConversationTaskID: conv("conv")}
}

func newChecker(f *fakeContainment) *ContainmentChecker {
	return NewContainmentChecker(f.readers(), nil)
}

func condition(t *testing.T, r ContainmentResult, name string) ContainmentCondition {
	t.Helper()
	if len(r.Conditions) != 4 {
		t.Fatalf("conditions = %d, want 4", len(r.Conditions))
	}
	for _, c := range r.Conditions {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no condition %q", name)
	return ContainmentCondition{}
}

func TestCheck_AllMetAndFixedOrder(t *testing.T) {
	r := newChecker(contained()).Check(context.Background(), testCoordinator())
	if !r.Contained {
		t.Fatalf("result = %+v, want contained", r)
	}
	want := []string{ConditionExecutorIsolated, ConditionAuthEnabled, ConditionNoKandevCredential, ConditionNoExtraTools}
	for i, c := range r.Conditions {
		if c.Name != want[i] || !c.Met {
			t.Fatalf("condition %d = %+v, want met %s", i, c, want[i])
		}
	}
}

func TestCheck_ExecutorIsolated(t *testing.T) {
	cases := []struct {
		typ  taskmodels.ExecutorType
		env  profiles.Environment
		met  bool
		name string
	}{
		{taskmodels.ExecutorTypeLocalDocker, profiles.EnvProd, true, "local_docker"},
		{taskmodels.ExecutorTypeRemoteDocker, profiles.EnvProd, true, "remote_docker"},
		{taskmodels.ExecutorTypeSprites, profiles.EnvProd, true, "sprites"},
		{taskmodels.ExecutorTypeKubernetes, profiles.EnvProd, true, "k8s"},
		{taskmodels.ExecutorTypeMockRemote, profiles.EnvE2E, true, "mock_remote e2e"},
		{taskmodels.ExecutorTypeMockRemote, profiles.EnvProd, false, "mock_remote prod"},
		{taskmodels.ExecutorTypeLocal, profiles.EnvProd, false, "local"},
		{taskmodels.ExecutorTypeWorktree, profiles.EnvProd, false, "worktree"},
		{taskmodels.ExecutorTypeSSH, profiles.EnvProd, false, "ssh"},
		{taskmodels.ExecutorTypePluginRemote, profiles.EnvProd, false, "plugin_remote"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := contained()
			f.executor.Type = tc.typ
			c := newChecker(f)
			c.environment = func() profiles.Environment { return tc.env }
			got := condition(t, c.Check(context.Background(), testCoordinator()), ConditionExecutorIsolated)
			if got.Met != tc.met || got.Detail != string(tc.typ) {
				t.Fatalf("got %+v, want met=%v detail=%s", got, tc.met, tc.typ)
			}
		})
	}
}

func TestCheck_ExecutorUnreadable(t *testing.T) {
	f := contained()
	f.execErr = errRead
	got := condition(t, newChecker(f).Check(context.Background(), testCoordinator()), ConditionExecutorIsolated)
	if got.Met || got.Detail != DetailUnreadable {
		t.Fatalf("got %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := newChecker(contained()).Check(ctx, testCoordinator())
	if res.Contained || condition(t, res, ConditionExecutorIsolated).Detail != DetailUnreadable {
		t.Fatalf("cancelled context = %+v", res)
	}
	nilRes := newChecker(contained()).Check(context.Background(), nil)
	if nilRes.Contained || len(nilRes.Conditions) != 4 {
		t.Fatalf("nil coordinator = %+v", nilRes)
	}
}

func TestCheck_AuthModes(t *testing.T) {
	for mode, want := range map[auth.Mode]struct {
		met    bool
		detail string
	}{auth.ModeEnabled: {true, ""}, auth.ModeDisabled: {false, "disabled"}, auth.ModeSetup: {false, "setup"}} {
		f := contained()
		f.mode = mode
		got := condition(t, newChecker(f).Check(context.Background(), testCoordinator()), ConditionAuthEnabled)
		if got.Met != want.met || got.Detail != want.detail {
			t.Fatalf("mode %s: %+v", mode, got)
		}
	}
}

func TestCheck_NoKandevCredentialSources(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fakeContainment)
		detail string
	}{
		{"executor key api", func(f *fakeContainment) {
			f.execProfile.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_API_KEY", Value: "x"}}
		}, "KANDEV_API_KEY"},
		{"agent key run token", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_RUN_TOKEN", Value: "x"}}
		}, "KANDEV_RUN_TOKEN"},
		{"literal pat value", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "OTHER", Value: "kandev_pat_abc"}}
		}, "pat_value"},
		{"secret pat value", func(f *fakeContainment) {
			f.execProfile.EnvVars = []taskmodels.ProfileEnvVar{{Key: "OTHER", SecretID: "s1"}}
			f.secrets["s1"] = fakeSecret{value: "kandev_pat_abc", updatedAt: tBefore}
		}, "pat_value"},
		{"provider api key secret", func(f *fakeContainment) {
			f.agent.ProviderAPIKeySecretID = "s1"
			f.secrets["s1"] = fakeSecret{value: "kandev_pat_abc", updatedAt: tBefore}
		}, "pat_value"},
		{"credentials manager", func(f *fakeContainment) {
			f.creds = []CredentialValue{{Key: "ANTHROPIC_API_KEY", Value: "kandev_pat_z"}}
		}, "pat_value"},
		{"pat not at start", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "OTHER", Value: "x kandev_pat_abc"}}
		}, ""},
		{"key is case sensitive", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "kandev_api_key", Value: "x"}}
		}, ""},
		{"key must be exact", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "MY_KANDEV_API_KEY", Value: "x"}}
		}, ""},
		{"secret unreadable", func(f *fakeContainment) {
			f.execProfile.EnvVars = []taskmodels.ProfileEnvVar{{Key: "OTHER", SecretID: "s1"}}
			f.secrets["s1"] = fakeSecret{err: errRead}
		}, DetailUnreadable},
		{"agent profile missing", func(f *fakeContainment) { f.agent = nil; f.agentErr = sql.ErrNoRows }, DetailUnreadable},
		{"credential lookup failed", func(f *fakeContainment) { f.credsErr = errRead }, DetailUnreadable},
		{"repository binding", func(f *fakeContainment) { f.bound = true }, DetailUnverifiedSource},
		{"repository read failed", func(f *fakeContainment) { f.boundErr = errRead }, DetailUnreadable},
		{"first source names detail", func(f *fakeContainment) {
			f.execProfile.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_RUN_TOKEN", Value: "x"}}
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_API_KEY", Value: "x"}}
		}, "KANDEV_RUN_TOKEN"},
		{"match outranks unverified", func(f *fakeContainment) {
			f.bound = true
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_API_KEY", Value: "x"}}
		}, "KANDEV_API_KEY"},
		{"unreadable outranks match", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_API_KEY", Value: "x"}}
			f.credsErr = errRead
		}, DetailUnreadable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := contained()
			tc.mutate(f)
			got := condition(t, newChecker(f).Check(context.Background(), testCoordinator()), ConditionNoKandevCredential)
			if got.Met != (tc.detail == "") || got.Detail != tc.detail {
				t.Fatalf("got %+v, want detail %q", got, tc.detail)
			}
		})
	}
}

func TestCheck_ANilReaderIsUnverifiedSource(t *testing.T) {
	for name, edit := range map[string]func(*ContainmentReaders){
		"credentials":  func(r *ContainmentReaders) { r.Credentials = nil },
		"repositories": func(r *ContainmentReaders) { r.Repositories = nil },
	} {
		t.Run(name, func(t *testing.T) {
			r := contained().readers()
			edit(&r)
			got := condition(t, NewContainmentChecker(r, nil).Check(context.Background(), testCoordinator()), ConditionNoKandevCredential)
			if got.Met || got.Detail != DetailUnverifiedSource {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestCheck_ConversationlessCoordinatorHasNoRepositorySource(t *testing.T) {
	r := contained().readers()
	r.Repositories = nil
	co := testCoordinator()
	co.ConversationTaskID = nil
	got := condition(t, NewContainmentChecker(r, nil).Check(context.Background(), co), ConditionNoKandevCredential)
	if !got.Met {
		t.Fatalf("got %+v", got)
	}
}

func TestCheck_NoExtraTools(t *testing.T) {
	cases := []struct {
		name   string
		mcp    *settingsmodels.AgentProfileMcpConfig
		err    error
		met    bool
		detail string
	}{
		{"no row", nil, sql.ErrNoRows, true, ""},
		{"read error", nil, errRead, false, DetailUnreadable},
		{"disabled with servers", &settingsmodels.AgentProfileMcpConfig{Enabled: false, Servers: map[string]interface{}{"a": 1}, UpdatedAt: tBefore}, nil, true, ""},
		{"enabled empty", &settingsmodels.AgentProfileMcpConfig{Enabled: true, UpdatedAt: tBefore}, nil, true, ""},
		{"one server", &settingsmodels.AgentProfileMcpConfig{Enabled: true, Servers: map[string]interface{}{"a": 1}, UpdatedAt: tBefore}, nil, false, "1"},
		{"kandev named server fails", &settingsmodels.AgentProfileMcpConfig{Enabled: true, Servers: map[string]interface{}{"kandev": 1, "b": 2}, UpdatedAt: tBefore}, nil, false, "2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := contained()
			f.mcp, f.mcpErr = tc.mcp, tc.err
			got := condition(t, newChecker(f).Check(context.Background(), testCoordinator()), ConditionNoExtraTools)
			if got.Met != tc.met || got.Detail != tc.detail {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestCheck_LaunchCurrency(t *testing.T) {
	launched := &ContainmentSession{State: "RUNNING", StartedAt: tLaunch}
	cases := []struct {
		name    string
		mutate  func(*fakeContainment)
		cond    string
		detail  string
		session *ContainmentSession
	}{
		{"executor profile edited", func(f *fakeContainment) { f.execProfile.UpdatedAt = tAfter }, ConditionNoKandevCredential, DetailChangedSinceLaunch, launched},
		{"agent profile edited", func(f *fakeContainment) { f.agent.UpdatedAt = tAfter }, ConditionNoKandevCredential, DetailChangedSinceLaunch, launched},
		{"revealed secret edited", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "A", SecretID: "s"}}
			f.secrets["s"] = fakeSecret{value: "v", updatedAt: tAfter}
		}, ConditionNoKandevCredential, DetailChangedSinceLaunch, launched},
		{"secret missing updated_at", func(f *fakeContainment) {
			f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "A", SecretID: "s"}}
			f.secrets["s"] = fakeSecret{value: "v"}
		}, ConditionNoKandevCredential, DetailUnreadable, launched},
		{"mcp row edited", func(f *fakeContainment) {
			f.mcp = &settingsmodels.AgentProfileMcpConfig{UpdatedAt: tAfter}
			f.mcpErr = nil
		}, ConditionNoExtraTools, DetailChangedSinceLaunch, launched},
		{"agent profile edit does not affect mcp when row exists", func(f *fakeContainment) {
			f.agent.UpdatedAt = tAfter
			f.mcp = &settingsmodels.AgentProfileMcpConfig{UpdatedAt: tBefore}
			f.mcpErr = nil
		}, ConditionNoExtraTools, "", launched},
		{"no mcp row uses agent profile", func(f *fakeContainment) { f.agent.UpdatedAt = tAfter }, ConditionNoExtraTools, DetailChangedSinceLaunch, launched},
		{"mcp row missing updated_at", func(f *fakeContainment) {
			f.mcp = &settingsmodels.AgentProfileMcpConfig{}
			f.mcpErr = nil
		}, ConditionNoExtraTools, DetailUnreadable, launched},
		{"created session is exempt by state", func(f *fakeContainment) { f.agent.UpdatedAt = tAfter }, ConditionNoKandevCredential, "",
			&ContainmentSession{State: launchStateCreated, StartedAt: tBefore}},
		{"created session is exempt for mcp", func(f *fakeContainment) { f.agent.UpdatedAt = tAfter }, ConditionNoExtraTools, "",
			&ContainmentSession{State: launchStateCreated}},
		{"no session", func(f *fakeContainment) { f.agent.UpdatedAt = tAfter }, ConditionNoKandevCredential, "", nil},
		{"session read failed", func(f *fakeContainment) { f.sessionErr = errRead }, ConditionNoExtraTools, DetailUnreadable, launched},
		{"unchanged sources", func(*fakeContainment) {}, ConditionNoKandevCredential, "", launched},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := contained()
			f.session = tc.session
			tc.mutate(f)
			got := condition(t, newChecker(f).Check(context.Background(), testCoordinator()), tc.cond)
			if got.Met != (tc.detail == "") || got.Detail != tc.detail {
				t.Fatalf("got %+v, want detail %q", got, tc.detail)
			}
		})
	}
}

func TestCheck_NeverCached(t *testing.T) {
	f := contained()
	c := newChecker(f)
	co := testCoordinator()
	if !c.Check(context.Background(), co).Contained {
		t.Fatal("first check must be contained")
	}
	f.mode = auth.ModeDisabled
	if c.Check(context.Background(), co).Contained {
		t.Fatal("a changed reader value must show on the next check")
	}
	if f.reads < 2 {
		t.Fatalf("reads = %d, want each check to read", f.reads)
	}
}

type fakeRecorder struct{ counts map[string]int }

func (r *fakeRecorder) ConditionFailed(c string) { r.counts[c]++ }

func TestCheckForAdmission_CountsAndLogsOnStateChange(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	f := contained()
	c := NewContainmentChecker(f.readers(), log)
	rec := &fakeRecorder{counts: map[string]int{}}
	co := testCoordinator()
	ctx := context.Background()

	c.CheckForAdmission(ctx, co, rec)
	if len(rec.counts) != 0 || logs.Len() != 0 {
		t.Fatalf("contained first call counts=%v logs=%d", rec.counts, logs.Len())
	}

	f.mode = auth.ModeDisabled
	f.executor.Type = taskmodels.ExecutorTypeLocal
	c.CheckForAdmission(ctx, co, rec)
	if rec.counts[ConditionExecutorIsolated] != 1 || rec.counts[ConditionAuthEnabled] != 1 {
		t.Fatalf("counts = %v, want each unmet condition once", rec.counts)
	}
	if logs.Len() != 1 {
		t.Fatalf("logs = %d, want 1 for the change to not contained", logs.Len())
	}

	c.CheckForAdmission(ctx, co, rec)
	if logs.Len() != 1 || rec.counts[ConditionAuthEnabled] != 2 {
		t.Fatalf("same key: logs=%d counts=%v", logs.Len(), rec.counts)
	}

	f.executor.Type = taskmodels.ExecutorTypeLocalDocker
	c.CheckForAdmission(ctx, co, rec)
	if logs.Len() != 2 {
		t.Fatalf("first unmet changed executor to auth, want a log, got %d", logs.Len())
	}

	f.agent.EnvVars = []taskmodels.ProfileEnvVar{{Key: "KANDEV_API_KEY", Value: "x"}}
	c.CheckForAdmission(ctx, co, rec)
	if logs.Len() != 2 {
		t.Fatalf("a later unmet condition must not log, got %d", logs.Len())
	}
}

func TestCheck_DirectCallMovesNeitherCounterNorLog(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	log, _ := logger.NewFromZap(zap.New(core))
	f := contained()
	f.mode = auth.ModeDisabled
	c := NewContainmentChecker(f.readers(), log)
	before := containmentFailedCount(ConditionAuthEnabled)
	c.Check(context.Background(), testCoordinator())
	if containmentFailedCount(ConditionAuthEnabled) != before || logs.Len() != 0 {
		t.Fatal("Check must not count or log")
	}
	if c.state.changed("co-1", admissionKey{}) {
		t.Fatal("Check must not set the previous key")
	}
}

func TestCheckForAdmission_DefaultRecorderCountsOnExpvar(t *testing.T) {
	f := contained()
	f.mode = auth.ModeDisabled
	before := containmentFailedCount(ConditionAuthEnabled)
	newChecker(f).CheckForAdmission(context.Background(), testCoordinator(), nil)
	if got := containmentFailedCount(ConditionAuthEnabled); got != before+1 {
		t.Fatalf("counter = %d, want %d", got, before+1)
	}
}
