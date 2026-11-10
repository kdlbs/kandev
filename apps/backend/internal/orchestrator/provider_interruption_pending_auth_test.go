package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startAgentManagedPendingContinuation(
	t *testing.T,
	caller authn.Identity,
) (*Service, *mockAgentManager, *transientRetryEntry) {
	t.Helper()
	svc, _, data := continuationFailureFixture(t)
	mgr := installContinuationRestoreFixture(t, svc)
	mgr.currentPromptExecutionID = data.AgentExecutionID
	mgr.currentPromptGeneration.Store(data.PromptGeneration - 1)
	mgr.getACPSessionIDForSessionFunc = func(string) (string, bool) {
		return "provider-session", true
	}
	svc.beginInteractivePromptAttempt(
		authn.WithIdentity(context.Background(), caller), data.SessionID, data.AgentExecutionID, false,
	)
	mgr.currentPromptGeneration.Store(data.PromptGeneration)
	svc.observePromptAttempt(data.SessionID, data.AgentExecutionID, data.PromptGeneration, true, true)
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = realPeerDisconnected
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: data.PromptGeneration,
		Known: true, Pending: true, PermissionsResolved: true, CompletedTools: 1,
	}
	require.NoError(t, svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "runtime-original", TaskID: data.TaskID, SessionID: data.SessionID,
		AgentExecutionID: data.AgentExecutionID, Status: "ready",
	}))
	require.True(t, svc.handleTransientFailure(context.Background(), data))
	value, ok := svc.transientRetries.Load(data.SessionID)
	require.True(t, ok)
	entry := value.(*transientRetryEntry)
	t.Cleanup(entry.cancel)
	require.Equal(t, continuationPolicyAgentManagedPendingLive, entry.continuationPolicy)
	return svc, mgr, entry
}

func productionShapedPromptChecker(
	caller authn.Identity,
	allowed *atomic.Bool,
	observe func(authn.Identity, bool),
) func(context.Context, string) error {
	return func(ctx context.Context, _ string) error {
		identity, known := authn.IdentityFromContext(ctx)
		if observe != nil {
			observe(identity, known)
		}
		// Task service scope checks allow auth-disabled, unscoped calls. Retries
		// must therefore preserve the initiating identity before they reach this
		// boundary.
		if !known {
			return nil
		}
		if identity != caller || !allowed.Load() {
			return errors.New("session.prompt denied")
		}
		return nil
	}
}

func TestAgentManagedPendingContinuationRechecksInitiatorAfterPermissionRevocation(t *testing.T) {
	caller := authn.Identity{UserID: "collaborator", Role: authn.RoleMember, OrgID: "org-1"}
	svc, mgr, entry := startAgentManagedPendingContinuation(t, caller)
	var allowed atomic.Bool
	allowed.Store(true)
	var sawCaller atomic.Bool
	svc.SetSessionPromptChecker(productionShapedPromptChecker(caller, &allowed, func(identity authn.Identity, known bool) {
		if known && identity == caller {
			sawCaller.Store(true)
		}
	}))

	allowed.Store(false) // The original permission is revoked during retry backoff.
	require.True(t, entry.claim())
	svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")

	mgr.mu.Lock()
	prompts := append([]string(nil), mgr.capturedPrompts...)
	mgr.mu.Unlock()
	assert.True(t, sawCaller.Load(), "the final check must see the identity that started the turn")
	assert.Zero(t, entry.started)
	assert.Empty(t, prompts, "a revoked collaborator must not dispatch the continuation")
}

func TestAgentManagedPendingContinuationRechecksInitiatorAtProviderAdmission(t *testing.T) {
	caller := authn.Identity{UserID: "collaborator", Role: authn.RoleMember, OrgID: "org-1"}
	svc, mgr, entry := startAgentManagedPendingContinuation(t, caller)
	var allowed atomic.Bool
	allowed.Store(true)
	var sawCaller atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	var checkOnce sync.Once
	svc.SetSessionPromptChecker(func(ctx context.Context, _ string) error {
		identity, known := authn.IdentityFromContext(ctx)
		if known && identity == caller {
			sawCaller.Store(true)
		}
		checkOnce.Do(func() {
			close(entered)
			<-release
		})
		if !known {
			return nil
		}
		if identity != caller || !allowed.Load() {
			return errors.New("session.prompt denied")
		}
		return nil
	})
	mgr.isAgentRunning = true
	mgr.isAgentReadyFn = func(context.Context, string) bool { return true }
	require.True(t, entry.claim())
	done := make(chan struct{})
	go func() {
		svc.retryTransientPrompt(entry.retryCtx, "t1", "s1", "execution-1")
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("pending-tool continuation did not reach provider admission")
	}
	allowed.Store(false)
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("revoked pending-tool continuation did not retire")
	}

	_, owned := svc.transientRetries.Load("s1")
	mgr.mu.Lock()
	prompts := append([]string(nil), mgr.capturedPrompts...)
	mgr.mu.Unlock()
	assert.True(t, sawCaller.Load(), "provider admission must recheck the initiating identity")
	assert.Zero(t, entry.started)
	assert.False(t, owned)
	assert.Empty(t, prompts, "revocation during the admission check must prevent dispatch")
}

func TestAgentManagedPendingContinuationRequiresBoundInitiatorWhenAuthorizationIsWired(t *testing.T) {
	svc, _, data := continuationFailureFixture(t)
	data.AgentProfileID = "profile-1"
	data.ErrorMessage = realPeerDisconnected
	data.PromptFailureDisposition = streams.PromptFailureDispositionRetainRuntime
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: data.PromptGeneration,
		Known: true, Pending: true, PermissionsResolved: true,
	}
	svc.SetSessionPromptChecker(func(context.Context, string) error { return nil })

	require.Nil(t, svc.continuationBindingForFailure(context.Background(), data),
		"a scoped authorization boundary requires a captured initiating user")
}

func TestAgentManagedPendingContinuationPreservesInitiatorAcrossSecondRetry(t *testing.T) {
	caller := authn.Identity{UserID: "collaborator", Role: authn.RoleMember, OrgID: "org-1"}
	svc, mgr, first := startAgentManagedPendingContinuation(t, caller)
	var allowed atomic.Bool
	allowed.Store(true)
	var observedMu sync.Mutex
	var observed []authn.Identity
	svc.SetSessionPromptChecker(productionShapedPromptChecker(caller, &allowed, func(identity authn.Identity, known bool) {
		observedMu.Lock()
		defer observedMu.Unlock()
		if known {
			observed = append(observed, identity)
		}
	}))

	require.True(t, first.claim())
	svc.retryTransientPrompt(first.retryCtx, "t1", "s1", "execution-1")
	data := first.retainedRuntime.failure
	data.PromptGeneration = mgr.currentPromptGeneration.Load()
	data.ContinuationSafety = &streams.ContinuationSafetySnapshot{
		Support: streams.ContinuationAgentManagedToolsV3, PromptGeneration: data.PromptGeneration,
		Known: true, Pending: true, PermissionsResolved: true, CompletedTools: 1,
	}
	require.Equal(t, uint64(8), data.PromptGeneration, "the first accepted continue advances the prompt generation")
	require.True(t, svc.handleTransientFailure(context.Background(), data),
		"a second retained V3 interruption remains in the same continuation episode")
	value, ok := svc.transientRetries.Load("s1")
	require.True(t, ok)
	second := value.(*transientRetryEntry)
	t.Cleanup(second.cancel)
	assert.True(t, second.continuation.initiatorKnown,
		"the accepted continuation must capture its initiating user for later retries")
	assert.Equal(t, caller, second.continuation.initiator)
	require.True(t, second.claim())
	svc.retryTransientPrompt(second.retryCtx, "t1", "s1", "execution-1")

	observedMu.Lock()
	defer observedMu.Unlock()
	assert.Equal(t, []authn.Identity{caller, caller}, observed,
		"both provider admission checks must receive the original caller")
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	require.Len(t, mgr.capturedPrompts, 2)
}
