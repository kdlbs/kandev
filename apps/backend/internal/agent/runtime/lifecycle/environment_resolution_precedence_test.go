package lifecycle

import (
	"context"
	"expvar"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// readOverrideCounter sums every environment_override_applied_total entry
// matching the given winning/losing origin label pair.
func readOverrideCounter(t *testing.T, winning, losing string) int64 {
	t.Helper()
	v := expvar.Get("environment_override_applied_total")
	if v == nil {
		return 0
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatalf("environment_override_applied_total is %T, want *expvar.Map", v)
	}
	label := "winning_origin=" + winning + ";losing_origin=" + losing
	var total int64
	m.Do(func(kv expvar.KeyValue) {
		if kv.Key != label {
			return
		}
		n, parseErr := strconv.ParseInt(kv.Value.String(), 10, 64)
		if parseErr != nil {
			t.Fatalf("counter %q value not int: %s", kv.Key, kv.Value.String())
		}
		total += n
	})
	return total
}

// TestResolveStrictEnvironment_EmitsOverrideLogAndCounter pins AC-23/AC-24:
// a launch that applies tier precedence to a key that would previously have
// conflicted emits one "environment override applied" Info log record and
// one envmetrics counter increment per losing origin.
func TestResolveStrictEnvironment_EmitsOverrideLogAndCounter(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)

	mgr := newTestManager(t)
	mgr.logger = log

	before := readOverrideCounter(t, runtimeenv.OriginManagedRuntime, runtimeenv.OriginAgentProfile)

	req := &LaunchRequest{
		TaskID: "task-1", SessionID: "session-1",
		EnvironmentResolutionRequired: true,
		EnvironmentDefinitions: []runtimeenv.Definition{
			{Key: "SHARED", Literal: "managed-value", Origin: runtimeenv.OriginManagedRuntime},
		},
	}
	profile := &AgentProfileInfo{
		EnvVars: []settingsmodels.ProfileEnvVar{{Key: "SHARED", Value: "profile-value"}},
	}

	env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, nil, profile)
	require.NoError(t, err)
	require.Equal(t, "managed-value", env["SHARED"], "Tier 1 must win over the Tier 2 profile literal")

	entries := logs.FilterMessage("environment override applied").All()
	require.Len(t, entries, 1, "exactly one OverrideRecord was produced")
	fields := entries[0].ContextMap()
	require.Equal(t, "SHARED", fields["env_key"])
	require.Equal(t, []interface{}{runtimeenv.OriginManagedRuntime}, fields["winning_origins"])
	require.EqualValues(t, int(runtimeenv.TierAuthoritative), fields["winning_tier"])
	require.Equal(t, []interface{}{runtimeenv.OriginAgentProfile}, fields["losing_origins"])
	require.EqualValues(t, []interface{}{int(runtimeenv.TierProfileDefault)}, fields["losing_tiers"])
	require.Equal(t, "task-1", fields["task_id"])
	require.Equal(t, "session-1", fields["session_id"])

	after := readOverrideCounter(t, runtimeenv.OriginManagedRuntime, runtimeenv.OriginAgentProfile)
	require.Equal(t, before+1, after)
}

// TestResolveStrictEnvironment_PreservesCapturedExecutorProfileDefinitions
// pins the executor-profile half of AC-42: resolveStrictEnvironment
// consumes req.EnvironmentDefinitions exactly as the orchestrator captured
// them and never re-reads the executor profile. m.executorProfileReader is
// left nil here — if this call chain re-read the executor profile it would
// panic on the nil reader instead of resolving from the captured
// definitions, so a clean resolve proves the capture is honoured.
func TestResolveStrictEnvironment_PreservesCapturedExecutorProfileDefinitions(t *testing.T) {
	mgr := newTestManager(t)
	require.Nil(t, mgr.executorProfileReader)

	req := &LaunchRequest{
		TaskID: "task-1", SessionID: "session-1",
		EnvironmentResolutionRequired: true,
		EnvironmentDefinitions: []runtimeenv.Definition{
			{Key: "TOKEN", Literal: "captured-value", Origin: runtimeenv.OriginExecutorProfile},
		},
	}

	env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, nil, nil)
	require.NoError(t, err)
	require.Equal(t, "captured-value", env["TOKEN"])
}

// TestBuildEnvForExecution_PreflightAcceptsButLifecycleResolveConflicts pins
// AC-43 using the F29-corrected construction. The spec's own literal
// wording — two differing Tier 1/Tier 2 literals — cannot produce this
// case: Tier 1 always wins that pair outright, which is this feature's
// headline behaviour, not a bug. Construction (a) instead uses a
// SECRET-BACKED agent-profile definition against a literal preflight key:
// the orchestrator preflight only ever sees the literal (validates
// cleanly), and the lifecycle site's later-appended secret-backed
// agent-profile definition trips the AC-9 secret veto the preflight could
// not see, so the launch fails on the RESOLVER's error, not the
// preflight's success.
func TestBuildEnvForExecution_PreflightAcceptsButLifecycleResolveConflicts(t *testing.T) {
	store := newInMemorySecretStore()
	require.NoError(t, store.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "secret-shared"}, Value: "secret-value",
	}))

	preflight := []runtimeenv.Definition{
		{Key: "SHARED", Literal: "preflight-value", Origin: runtimeenv.OriginExecutorProfile},
	}
	require.NoError(t, runtimeenv.Validate(preflight), "the preflight must accept a single Tier 1 literal")

	mgr := newTestManager(t)
	mgr.secretStore = store
	req := &LaunchRequest{
		TaskID: "task-1", SessionID: "session-1",
		EnvironmentResolutionRequired: true,
		EnvironmentDefinitions:        preflight,
	}
	profile := &AgentProfileInfo{
		EnvVars: []settingsmodels.ProfileEnvVar{{Key: "SHARED", SecretID: "secret-shared"}},
	}

	_, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, nil, profile)
	require.Error(t, err)
	var conflictErr *runtimeenv.ConflictError
	require.ErrorAs(t, err, &conflictErr)
}

// TestExecutorProfileEnvironmentDefinitions_SkipsBlankEntry is the
// lifecycle-side half of AC-38b's dual-path equivalence requirement: given
// the identical ProfileEnvVar input used by
// TestTaskEnvironmentSources_SkipsBlankProfileEnvVar in the orchestrator
// package, executorProfileEnvironmentDefinitions produces the same
// definition set — one definition, for the populated key only. This path
// already dropped blanks before this feature
// (environment_resolution.go:257-259 pre-change); the orchestrator path is
// what this feature changed, and both must now agree.
func TestExecutorProfileEnvironmentDefinitions_SkipsBlankEntry(t *testing.T) {
	reader := &fakeExecutorProfileReader{
		profiles: map[string]*models.ExecutorProfile{
			"prof-1": {
				ID: "prof-1",
				EnvVars: []models.ProfileEnvVar{
					{Key: "POPULATED", Value: "value"},
					{Key: "BLANK"},
				},
			},
		},
	}
	mgr := newExecutorProfileEnvManager(t, reader)

	defs, err := mgr.executorProfileEnvironmentDefinitions(context.Background(), "prof-1")
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, "POPULATED", defs[0].Key)
}
