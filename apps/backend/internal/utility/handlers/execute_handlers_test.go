package handlers

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/utility/dto"
	"github.com/kandev/kandev/internal/utility/models"
)

// executePrompt POSTs to /utility/execute and decodes the response.
func executePrompt(t *testing.T, f *utilityFixture, body string) (int, dto.ExecutePromptResponse) {
	t.Helper()
	status, raw := do(t, f, http.MethodPost, "/api/v1/utility/execute", body)
	var got dto.ExecutePromptResponse
	decodeInto(t, raw, &got)
	return status, got
}

// seedExecutableAgent adds a runnable custom utility agent whose prompt
// interpolates one template variable.
func seedExecutableAgent(f *utilityFixture) *models.UtilityAgent {
	return seedUtilityAgent(f, &models.UtilityAgent{
		ID: "custom-1", Name: "Reviewer", Prompt: "Review this: {{GitDiff}}",
		AgentID: "codex-acp", Model: "gpt-5", Enabled: true,
	})
}

// TestExecutePromptSessionSuccess pins the full success contract of the
// session-bound path: the resolved prompt that reached the executor, the
// response body, and the completed call record. A test that only checked the
// 200 would pass with the response fields dropped.
func TestExecutePromptSessionSuccess(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{
		executor: &stubInferenceExecutor{resp: &agentctlutil.PromptResponse{
			Success: true, Response: "looks good", Model: "gpt-5",
			PromptTokens: 12, ResponseTokens: 34, DurationMs: 56,
		}},
	})
	seedExecutableAgent(f)

	status, got := executePrompt(t, f,
		`{"utility_agent_id":"custom-1","session_id":"session-1","git_diff":"+one line"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %#v", status, got)
	}
	if got.CallID == "" {
		t.Fatal("response carries no call id")
	}
	want := dto.ExecutePromptResponse{
		Success: true, CallID: got.CallID, Response: "looks good", Model: "gpt-5",
		PromptTokens: 12, ResponseTokens: 34, DurationMs: 56,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}

	if len(f.executor.calls) != 1 {
		t.Fatalf("executor invocations = %#v, want 1", f.executor.calls)
	}
	call := f.executor.calls[0]
	wantCall := inferenceCall{
		sessionID: "session-1", agentID: "codex-acp", model: "gpt-5",
		prompt: "Review this: +one line",
	}
	if call != wantCall {
		t.Fatalf("executor received %#v, want %#v", call, wantCall)
	}

	record := f.repo.call(got.CallID)
	if record == nil {
		t.Fatal("no call record stored")
	}
	if record.Status != "completed" || record.Response != "looks good" ||
		record.PromptTokens != 12 || record.ResponseTokens != 34 || record.DurationMs != 56 {
		t.Fatalf("call record = %#v", record)
	}
	if record.CompletedAt == nil {
		t.Fatal("completed call record has no completion timestamp")
	}
}

// TestExecutePromptSessionFailures covers the two ways a session-bound run can
// fail. They are deliberately different statuses: a transport error is a 500,
// while an agent that ran and reported failure is a 200 carrying the reason —
// and both must leave the call record marked failed rather than pending.
func TestExecutePromptSessionFailures(t *testing.T) {
	t.Run("executor error is 500 and fails the call", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{
			executor: &stubInferenceExecutor{err: errors.New("agentctl unreachable")},
		})
		seedExecutableAgent(f)

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
		if status != http.StatusInternalServerError {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if got.Success || got.Error != "failed to execute prompt: agentctl unreachable" {
			t.Fatalf("response = %#v", got)
		}
		record := f.repo.call(got.CallID)
		if record == nil || record.Status != "failed" || record.ErrorMessage != "agentctl unreachable" {
			t.Fatalf("call record = %#v", record)
		}
	})

	t.Run("agent-reported failure is 200 carrying the reason", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{
			executor: &stubInferenceExecutor{resp: &agentctlutil.PromptResponse{
				Success: false, Error: "model refused", DurationMs: 7,
			}},
		})
		seedExecutableAgent(f)

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		want := dto.ExecutePromptResponse{CallID: got.CallID, Error: "model refused", DurationMs: 7}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("response = %#v, want %#v", got, want)
		}
		record := f.repo.call(got.CallID)
		if record == nil || record.Status != "failed" || record.DurationMs != 7 {
			t.Fatalf("call record = %#v", record)
		}
	})
}

// TestExecutePromptRejections covers the rejections that happen before any
// execution, and asserts no call record is written for them — a pending row
// with no run behind it shows up in the call history as a stuck invocation.
func TestExecutePromptRejections(t *testing.T) {
	t.Run("malformed payload", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		status, got := executePrompt(t, f, "{")
		if status != http.StatusBadRequest || got.Error != "invalid payload" {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if f.repo.callCount() != 0 {
			t.Fatalf("rejected request wrote %d call records", f.repo.callCount())
		}
	})

	t.Run("missing utility_agent_id", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		status, got := executePrompt(t, f, `{"session_id":"session-1"}`)
		if status != http.StatusBadRequest || got.Error != "invalid payload" {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
	})

	t.Run("unknown utility agent is 404", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		status, got := executePrompt(t, f, `{"utility_agent_id":"nope","session_id":"session-1"}`)
		if status != http.StatusNotFound || got.Error != "failed to prepare prompt" {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if f.repo.callCount() != 0 {
			t.Fatalf("404 request wrote %d call records", f.repo.callCount())
		}
	})

	t.Run("unresolvable inference agent is 400 before any call record", func(t *testing.T) {
		// No agent_id on the row and no user default to fall back on, so the
		// request cannot name an agent to run.
		f := newUtilityFixture(t, utilityFixtureOptions{noSettings: true})
		seedUtilityAgent(f, &models.UtilityAgent{ID: "custom-1", Name: "Reviewer", Prompt: "Review"})

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if got.Error != lifecycle.ErrInferenceAgentIDRequired.Error() {
			t.Fatalf("error = %q, want %q", got.Error, lifecycle.ErrInferenceAgentIDRequired.Error())
		}
		if got.CallID != "" || f.repo.callCount() != 0 {
			t.Fatalf("missing-agent rejection wrote call %q (%d records)", got.CallID, f.repo.callCount())
		}
		if len(f.executor.calls) != 0 {
			t.Fatalf("missing-agent rejection still executed %#v", f.executor.calls)
		}
	})

	t.Run("call-record write failure is 500 and never executes", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		seedExecutableAgent(f)
		f.repo.failWith("CreateCall", errors.New("database is locked"))

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
		if status != http.StatusInternalServerError || got.Error != "failed to create call record" {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if len(f.executor.calls) != 0 {
			t.Fatalf("execution ran despite the failed call record: %#v", f.executor.calls)
		}
	})
}

// TestExecutePromptUsesUserDefaults pins that the user's default agent/model
// pair is applied as a unit when the utility agent names neither: sending a
// default model from one provider to another provider's CLI is exactly the
// mix-up the pairing rule exists to prevent.
func TestExecutePromptUsesUserDefaults(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{
		settings: &stubUserSettings{agentID: "claude-acp", model: "sonnet"},
	})
	seedUtilityAgent(f, &models.UtilityAgent{
		ID: "builtin-1", Name: "Commit", Prompt: "Summarize", Builtin: true, Enabled: true,
		ProfileBindingState: models.ProfileBindingInherit,
	})

	status, got := executePrompt(t, f, `{"utility_agent_id":"builtin-1","session_id":"session-1"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %#v", status, got)
	}
	if len(f.executor.calls) != 1 {
		t.Fatalf("executor invocations = %#v", f.executor.calls)
	}
	if f.executor.calls[0].agentID != "claude-acp" || f.executor.calls[0].model != "sonnet" {
		t.Fatalf("executor received %#v, want the default agent/model pair", f.executor.calls[0])
	}
	if record := f.repo.call(got.CallID); record == nil || record.Model != "sonnet" {
		t.Fatalf("call record = %#v, want the default model persisted", record)
	}
}

// TestExecutePromptSessionless covers the no-session path used by flows like
// "enhance prompt" in the new-task modal: it runs through the host utility, and
// an unconfigured host is a 503 rather than a generic failure.
func TestExecutePromptSessionless(t *testing.T) {
	t.Run("runs through the host utility and completes the call", func(t *testing.T) {
		host := newStatefulHostStub()
		host.promptResult = &hostutility.PromptResult{
			Response: "enhanced", Model: "gpt-5", PromptTokens: 3, ResponseTokens: 4, DurationMs: 5,
		}
		f := newUtilityFixture(t, utilityFixtureOptions{host: host})
		seedExecutableAgent(f)

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","git_diff":"+one line"}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		want := dto.ExecutePromptResponse{
			Success: true, CallID: got.CallID, Response: "enhanced", Model: "gpt-5",
			PromptTokens: 3, ResponseTokens: 4, DurationMs: 5,
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("response = %#v, want %#v", got, want)
		}
		if len(host.prompts) != 1 {
			t.Fatalf("host invocations = %#v, want 1", host.prompts)
		}
		wantPrompt := hostPrompt{agentType: "codex-acp", model: "gpt-5", prompt: "Review this: +one line"}
		if host.prompts[0] != wantPrompt {
			t.Fatalf("host received %#v, want %#v", host.prompts[0], wantPrompt)
		}
		if len(f.executor.calls) != 0 {
			t.Fatalf("sessionless run also hit the session executor: %#v", f.executor.calls)
		}
		if record := f.repo.call(got.CallID); record == nil || record.Status != "completed" {
			t.Fatalf("call record = %#v", record)
		}
	})

	t.Run("host failure is 500 and fails the call", func(t *testing.T) {
		host := newStatefulHostStub()
		host.promptErr = errors.New("host agent not installed")
		f := newUtilityFixture(t, utilityFixtureOptions{host: host})
		seedExecutableAgent(f)

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1"}`)
		if status != http.StatusInternalServerError {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if got.Error != "failed to execute prompt: host agent not installed" {
			t.Fatalf("error = %q", got.Error)
		}
		if record := f.repo.call(got.CallID); record == nil || record.Status != "failed" {
			t.Fatalf("call record = %#v", record)
		}
	})

	t.Run("no host utility is 503 telling the client to supply a session", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{nilHost: true})
		seedExecutableAgent(f)

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1"}`)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if got.Error != "host utility not configured; session_id is required" {
			t.Fatalf("error = %q", got.Error)
		}
		record := f.repo.call(got.CallID)
		if record == nil || record.Status != "failed" || record.ErrorMessage != "host utility not configured" {
			t.Fatalf("call record = %#v", record)
		}
	})
}

// TestExecutePromptProfileBound covers the profile-resolution path. Neither
// stub executor implements the profile-aware interface, so both transports
// must report the capability as unavailable (503) instead of silently
// executing with the legacy agent/model pair — which would run the prompt
// against a provider the profile did not select.
func TestExecutePromptProfileBound(t *testing.T) {
	resolver := stubProfileResolver{profile: &agentsettingsmodels.AgentProfile{
		ID: "profile-1", AgentID: "claude-acp", Model: "sonnet",
	}}

	t.Run("session path without a profile-aware executor is 503", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{resolver: resolver})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "custom-1", Name: "Reviewer", Prompt: "Review", Enabled: true,
			AgentProfileID: "profile-1", ProfileBindingState: models.ProfileBindingExplicit,
		})

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if got.Error != "profile-aware inference executor is unavailable" {
			t.Fatalf("error = %q", got.Error)
		}
		if len(f.executor.calls) != 0 {
			t.Fatalf("fell back to the legacy executor: %#v", f.executor.calls)
		}
		if record := f.repo.call(got.CallID); record == nil || record.Status != "failed" {
			t.Fatalf("call record = %#v", record)
		}
	})

	t.Run("sessionless path without a profile-aware host is 503", func(t *testing.T) {
		host := newStatefulHostStub()
		f := newUtilityFixture(t, utilityFixtureOptions{resolver: resolver, host: host})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "custom-1", Name: "Reviewer", Prompt: "Review", Enabled: true,
			AgentProfileID: "profile-1", ProfileBindingState: models.ProfileBindingExplicit,
		})

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1"}`)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if got.Error != "profile-aware host utility executor is unavailable" {
			t.Fatalf("error = %q", got.Error)
		}
		if len(host.prompts) != 0 {
			t.Fatalf("fell back to the legacy host prompt: %#v", host.prompts)
		}
	})

	t.Run("an unconfigured binding never reaches an executor", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{resolver: resolver})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "custom-1", Name: "Reviewer", Prompt: "Review", Enabled: true,
			ProfileBindingState: models.ProfileBindingUnconfigured,
		})

		status, got := executePrompt(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
		if status != http.StatusInternalServerError || got.Error != "failed to prepare prompt" {
			t.Fatalf("status = %d, body = %#v", status, got)
		}
		if f.repo.callCount() != 0 || len(f.executor.calls) != 0 {
			t.Fatalf("unconfigured binding wrote %d calls and executed %#v",
				f.repo.callCount(), f.executor.calls)
		}
	})
}
