import { claimSessionRecovery } from "@/hooks/domains/session/session-recovery-pending";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { Message, TaskSession } from "@/lib/types/http";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryProvider, useSessionComposerRecovery } from "./session-recovery-context";
import { SessionRecoveryCard } from "./session-recovery-card";
import { ActionMessage } from "./messages/action-message";

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
afterEach(cleanup);
const RECOVERY_CARD = "session-recovery-card";
const NPM_POLICY = "managed_runtime_npm_policy";
const CONNECTION_LOST = "Connection lost";
const resume = vi.fn();
const relocate = vi.fn().mockResolvedValue(true);
const actions = {
  busyAction: null,
  recoveryError: null,
  guardDetails: null,
  branchDetails: null,
  recoveryNotice: null,
  managedCloneRecoveryStamp: null,
  handleRecover: resume,
  handleManagedCloneRelocation: relocate,
} as unknown as SessionRecoveryActions;
const session = {
  id: "session",
  task_id: "task",
  state: "FAILED",
  agent_profile_id: "profile",
  error_message: CONNECTION_LOST,
  metadata: { last_agent_error: { message: CONNECTION_LOST, stamp: "failure" } },
} as unknown as TaskSession;
function message(kind?: string): Message {
  return {
    id: "failure",
    session_id: "session",
    task_id: "task",
    type: "status",
    author_type: "agent",
    content: CONNECTION_LOST,
    created_at: "2026-09-20T10:00:00Z",
    metadata: {
      recovery_actions: true,
      error_stamp: "failure",
      failure_kind: kind,
      provider_name: "OpenCode",
      error_output: "Authorization: Bearer hidden-fixture-value",
    },
  } as unknown as Message;
}
function Owner() {
  const context = useSessionComposerRecovery("session");
  return context?.model ? (
    <SessionRecoveryCard model={context.model} actions={actions} onNewSession={vi.fn()} />
  ) : null;
}
function ownerView(current: TaskSession) {
  return (
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session: current } },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryProvider session={current} messages={[]} taskId="task" enabled>
        <Owner />
      </SessionRecoveryProvider>
    </StateProvider>
  );
}
function renderCase(kind?: string) {
  const row = message(kind);
  return render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session } },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryProvider session={session} messages={[row]} taskId="task" enabled>
        <ActionMessage comment={row} />
        <Owner />
      </SessionRecoveryProvider>
    </StateProvider>,
  );
}
describe("composer recovery ownership", () => {
  it("renders one active owner and history without duplicate controls", () => {
    renderCase();
    expect(screen.getAllByTestId(RESUME_BUTTON)).toHaveLength(1);
    expect(
      screen
        .getByTestId("session-recovery-history")
        .querySelector(`[data-testid="${RESUME_BUTTON}"]`),
    ).toBeNull();
    expect(screen.getByTestId(FRESH_BUTTON)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "More options" })).toBeNull();
    expect(screen.getByTestId("session-recovery-history").textContent).toContain(
      "This failure is explained in the recovery card above.",
    );
    expect(screen.getByTestId("session-recovery-history").textContent).not.toContain(
      CONNECTION_LOST,
    );
    fireEvent.click(screen.getByTestId(RESUME_BUTTON));
    expect(resume).toHaveBeenCalledWith("resume");
    expect(document.body.textContent).not.toContain("hidden-fixture-value");
  });
  it.each(["managed_runtime_npm_resolution", NPM_POLICY])(
    "uses runtime retry for %s in the same card instead of resume",
    (kind) => {
      renderCase(kind);
      if (kind === NPM_POLICY)
        expect(screen.getByTestId(RECOVERY_CARD).textContent).toContain(
          "npm blocked this runtime version",
        );
      expect(screen.getByTestId(RECOVERY_CARD)).toBeTruthy();
      expect(screen.queryByTestId(RESUME_BUTTON)).toBeNull();
      fireEvent.click(screen.getByTestId("managed-runtime-npm-retry-button"));
      expect(resume).toHaveBeenCalledWith("runtime_retry");
    },
  );
  it("offers manual resume for quota without implying fresh sessions reset capacity", () => {
    renderCase("provider_quota_limited");
    expect(screen.getByTestId(RECOVERY_CARD)).toBeTruthy();
    expect(screen.queryByTestId("provider-quota-recovery")).toBeNull();
    fireEvent.click(screen.getByTestId(RESUME_BUTTON));
    expect(resume).toHaveBeenCalledWith("resume");
    expect(screen.queryByTestId(FRESH_BUTTON)).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete task" })).toBeNull();
  });

  it("replaces a cancelled legacy error with one relocation action after projection", () => {
    const cancelled = {
      ...session,
      state: "CANCELLED",
      error_message: "The previous agent launch failed.",
      metadata: {
        last_agent_error: {
          message: "The previous agent launch failed.",
          stamp: "legacy-generic-stamp",
          scope: "session",
        },
      },
    } as unknown as TaskSession;
    const { rerender } = render(ownerView(cancelled));
    expect(screen.queryByTestId(RECOVERY_CARD)).toBeNull();

    const projected = {
      ...cancelled,
      metadata: {
        last_agent_error: {
          message: "Workspace needs repair",
          occurred_at: "2026-10-02T00:00:00Z",
          stamp: "managed-clone-stamp",
          scope: "session",
          code: "managed_clone_relocation_required",
          recovery_actions: ["relocate_and_resume"],
        },
      },
    } as unknown as TaskSession;
    rerender(ownerView(projected));

    expect(screen.getAllByTestId("managed-clone-relocate-button")).toHaveLength(1);
    expect(screen.queryByTestId(RESUME_BUTTON)).toBeNull();
    expect(screen.queryByTestId(FRESH_BUTTON)).toBeNull();
    expect(screen.queryByTestId(RESTORE_WORKSPACE_BUTTON)).toBeNull();
  });

  it("keeps one disabled action group and one live status while recovery waits", () => {
    const pendingActions = { ...actions, busyAction: "resume" } as SessionRecoveryActions;
    const { container } = render(
      <StateProvider
        initialState={
          {
            taskSessions: { items: { session } },
            agentProfiles: { items: [{ id: "profile" }] },
          } as unknown as Partial<AppState>
        }
      >
        <SessionRecoveryCard
          model={{ sessionId: "session", stamp: "failure", kind: "generic" }}
          actions={pendingActions}
          onNewSession={vi.fn()}
        />
      </StateProvider>,
    );

    expect(container.querySelectorAll('[aria-busy="true"]')).toHaveLength(1);
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.getByRole("status").textContent).toBe("Resuming...");
    expect(screen.getByTestId(RESUME_BUTTON)).toHaveProperty("disabled", true);
    expect(screen.getByTestId(FRESH_BUTTON)).toHaveProperty("disabled", true);
  });
});

it("preserves bootstrap restore eligibility and separate cause details", () => {
  render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session } },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          kind: "generic",
          error: {
            message: "Could not start",
            phase: "bootstrap",
            causes: [{ operation: "resume", code: "permission_denied", detail: "original cause" }],
          },
        }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  expect(screen.getByTestId(RESTORE_WORKSPACE_BUTTON)).toBeTruthy();
  expect(screen.getByTestId(FRESH_BUTTON)).toBeTruthy();
  fireEvent.click(screen.getByText("Technical details"));
  expect(document.body.textContent).toContain("original cause");
});

describe("read-only recovery presentation", () => {
  it("does not repeat the read-only status below its title", () => {
    const notice = "Workspace restored in read-only mode";
    render(
      <StateProvider
        initialState={
          {
            taskSessions: { items: { session } },
            agentProfiles: { items: [{ id: "profile" }] },
          } as unknown as Partial<AppState>
        }
      >
        <SessionRecoveryCard
          model={{
            sessionId: "session",
            kind: "generic",
            error: { message: "Workspace recovery failed.", phase: "bootstrap" },
          }}
          actions={{ ...actions, recoveryNotice: notice, manualRecoveryFailure: null }}
          onNewSession={vi.fn()}
        />
      </StateProvider>,
    );

    const title = screen.getByRole("heading", { name: notice });
    expect(title.getAttribute("aria-live")).toBe("polite");
    expect(title.getAttribute("aria-atomic")).toBe("true");
    expect(screen.getAllByText(notice)).toHaveLength(1);
  });

  it("keeps a distinct recovery failure visible and announced", () => {
    const recoveryError = new Error("The provider rejected the restored session.");
    render(
      <StateProvider
        initialState={
          {
            taskSessions: { items: { session } },
            agentProfiles: { items: [{ id: "profile" }] },
          } as unknown as Partial<AppState>
        }
      >
        <SessionRecoveryCard
          model={{
            sessionId: "session",
            kind: "generic",
            error: { message: "Workspace recovery failed.", phase: "bootstrap" },
          }}
          actions={{
            ...actions,
            recoveryError,
            recoveryNotice: null,
            manualRecoveryFailure: {
              operation: "resume",
              sessionId: "session",
              errorStamp: null,
              requestKey: "task\u0000session\u0000",
              operationId: 1,
            },
          }}
          onNewSession={vi.fn()}
        />
      </StateProvider>,
    );

    const error = screen.getByTestId("session-recovery-error");
    expect(error.getAttribute("role")).toBe("status");
    expect(error.textContent).toContain("Failed to resume session");
  });
});

it("keeps typed selection copy primary over a correlated manual recovery error", () => {
  const stamp = "bootstrap-model-1";
  render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session } },
          agentProfiles: { items: [{ id: "profile", agent_name: "auggie" }] },
          availableAgents: { items: [{ name: "auggie", display_name: "Auggie" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          stamp,
          kind: "generic",
          details: "agent_bootstrap; cause=model_unavailable",
          error: {
            message: "The agent could not start.",
            occurredAt: "2026-09-30T11:00:00Z",
            phase: "bootstrap",
            causes: [
              {
                operation: "start",
                code: "model_unavailable",
                reason: "requested_not_advertised",
                requested_model: "claude-opus-4-8",
                prompt_not_sent: true,
              },
            ],
          },
        }}
        actions={{
          ...actions,
          recoveryError: new Error("The later resume request failed."),
          manualRecoveryFailure: {
            operation: "resume",
            sessionId: "session",
            errorStamp: stamp,
            requestKey: `task\u0000session\u0000${stamp}`,
            operationId: 1,
          },
        }}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );

  expect(screen.getByRole("heading", { name: "Saved model unavailable" })).toBeTruthy();
  expect(screen.getByText(/did not list "claude-opus-4-8"/)).toBeTruthy();
  expect(screen.getByTestId("session-bootstrap-no-prompt").textContent).toBe("No prompt was sent.");
  expect(screen.getByTestId("session-recovery-fresh-start-warning").textContent).toContain(
    "A fresh session uses your saved selections",
  );
  expect(screen.queryByText("Failed to resume session")).toBeNull();
  fireEvent.click(screen.getByText("Technical details"));
  const details = screen.getByText(/Requested model/).textContent ?? "";
  expect(details).toContain("claude-opus-4-8");
  expect(details).toContain("The later resume request failed.");
  expect(details).not.toContain("agent_bootstrap; cause=model_unavailable");
});

it("keeps a typed startup cause and workspace status visible after read-only restore", () => {
  render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session } },
          agentProfiles: { items: [{ id: "profile", agent_name: "auggie" }] },
          availableAgents: { items: [{ name: "auggie", display_name: "Auggie" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          stamp: "typed-readonly",
          kind: "generic",
          error: {
            message: "The agent could not start.",
            phase: "bootstrap",
            causes: [
              {
                operation: "start",
                code: "model_unavailable",
                reason: "requested_not_advertised",
                requested_model: "unlisted-model",
                prompt_not_sent: true,
              },
            ],
          },
        }}
        actions={{
          ...actions,
          recoveryNotice: "Workspace restored in read-only mode",
          manualRecoveryFailure: null,
        }}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );

  expect(screen.getByRole("heading", { name: "Saved model unavailable" })).toBeTruthy();
  expect(screen.getByText(/did not list "unlisted-model"/)).toBeTruthy();
  expect(screen.getByTestId("session-recovery-workspace-status").textContent).toContain(
    "read-only mode",
  );
  expect(screen.getByTestId("recovery-resume-button")).toBeTruthy();
  expect(screen.getByTestId("recovery-fresh-button")).toBeTruthy();
});

it("shows only the confirmed managed clone relocation action", () => {
  render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session } },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          stamp: "managed-stamp",
          kind: "managed_clone_relocation_required",
          summary: "old clone mismatch",
        }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );

  expect(screen.getByTestId("managed-clone-relocate-button")).toBeTruthy();
  expect(screen.queryByTestId(RESUME_BUTTON)).toBeNull();
  expect(screen.queryByTestId(FRESH_BUTTON)).toBeNull();
  expect(screen.queryByTestId(RESTORE_WORKSPACE_BUTTON)).toBeNull();
  fireEvent.click(screen.getByTestId("managed-clone-relocate-button"));
  expect(screen.getByTestId("managed-clone-relocation-confirm")).toBeTruthy();
  expect(document.body.textContent).toContain("Git staging choices do not transfer");
  fireEvent.click(screen.getByTestId("managed-clone-relocation-confirm"));
  expect(relocate).toHaveBeenCalledOnce();
});

const RESUME_BUTTON = "recovery-resume-button";
const FRESH_BUTTON = "recovery-fresh-button";
const RESTORE_WORKSPACE_BUTTON = "recovery-restore-workspace-button";

function PendingProbe() {
  const context = useSessionComposerRecovery("session");
  return (
    <span data-testid="pending-probe">
      {context?.model ? (context.pending ?? "ready") : "healthy"}
    </span>
  );
}
it("keeps the same operation pending through STARTING, then releases recovery on success", () => {
  const view = (state: TaskSession["state"]) => (
    <SessionRecoveryProvider
      session={{
        ...session,
        state,
        error_message: state === "WAITING_FOR_INPUT" ? "" : session.error_message,
      }}
      messages={[]}
      taskId="task"
      enabled
    >
      <PendingProbe />
    </SessionRecoveryProvider>
  );
  const { rerender } = render(view("FAILED"));
  let release: (() => void) | null = null;
  act(() => {
    release = claimSessionRecovery("task\u0000session", "runtime_retry");
  });
  expect(screen.getByTestId("pending-probe").textContent).toBe("runtime_retry");
  rerender(view("STARTING"));
  act(() => {
    release?.();
  });
  expect(screen.getByTestId("pending-probe").textContent).toBe("runtime_retry");
  rerender(view("WAITING_FOR_INPUT"));
  expect(screen.getByTestId("pending-probe").textContent).toBe("healthy");
});

it("shows disabled recovery choices while history loads, then enables them", () => {
  resume.mockClear();
  const view = (loading: boolean) => (
    <StateProvider>
      <SessionRecoveryCard
        model={{ sessionId: "session", kind: "generic", loading }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>
  );
  const { rerender } = render(view(true));
  const fresh = screen.getByTestId(FRESH_BUTTON);
  expect(screen.getByTestId(RESUME_BUTTON)).toBeTruthy();
  expect(fresh).toHaveProperty("disabled", true);
  fireEvent.click(fresh);
  expect(resume).not.toHaveBeenCalled();
  rerender(view(false));
  expect(screen.getByTestId(FRESH_BUTTON)).toBe(fresh);
  expect(fresh).toHaveProperty("disabled", false);
});

it.each(["managed_runtime_npm_resolution", NPM_POLICY, "provider_quota_limited"])(
  "preserves specialized %s actions while history loads",
  (kind) => {
    render(
      <StateProvider>
        <SessionRecoveryCard
          model={{ sessionId: "session", kind, loading: true }}
          actions={actions}
          onNewSession={vi.fn()}
        />
      </StateProvider>,
    );
    if (kind === "provider_quota_limited")
      expect(screen.getByTestId(RESUME_BUTTON)).toHaveProperty("disabled", true);
    else expect(screen.queryByTestId(RESUME_BUTTON)).toBeNull();
    expect(screen.queryByTestId(FRESH_BUTTON)).toBeNull();
    if (kind !== "provider_quota_limited")
      expect(screen.getByTestId("managed-runtime-npm-retry-button")).toHaveProperty(
        "disabled",
        true,
      );
  },
);

it.each(["managed_runtime_npm_resolution", NPM_POLICY])(
  "keeps %s startup on its specialized retry operation",
  (kind) => {
    render(
      <StateProvider>
        <SessionRecoveryCard
          model={{
            sessionId: "session",
            kind,
            error: { message: "npm failed", phase: "bootstrap" },
          }}
          actions={actions}
          onNewSession={vi.fn()}
        />
      </StateProvider>,
    );
    expect(screen.getByTestId("managed-runtime-npm-retry-button")).toBeTruthy();
    expect(screen.queryByTestId(RESTORE_WORKSPACE_BUTTON)).toBeNull();
    expect(screen.queryByTestId(RESUME_BUTTON)).toBeNull();
  },
);

it.each([false, true])("honors explicit quota action metadata (resume: %s)", (allowResume) => {
  render(
    <StateProvider>
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          kind: "provider_quota_limited",
          metadata: {
            actions: allowResume
              ? [
                  {
                    type: "ws_request",
                    label: "Resume",
                    params: { method: "session.recover", payload: { action: "resume" } },
                  },
                ]
              : [],
          },
        }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  expect(Boolean(screen.queryByTestId(RESUME_BUTTON))).toBe(allowResume);
  expect(screen.queryByTestId(FRESH_BUTTON)).toBeNull();
});

it("preserves the provider remediation link in the active card", () => {
  render(
    <StateProvider>
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          kind: "provider_quota_limited",
          metadata: { remediation_url: "https://opencode.ai/workspace/demo/go" },
        }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  expect(screen.getByTestId("remediation-link").getAttribute("href")).toBe(
    "https://opencode.ai/workspace/demo/go",
  );
});
it("preserves the backend's fresh-first recommendation and resume warning", () => {
  const metadata = {
    actions: ["fresh_start", "resume"].map((action) => ({
      type: "ws_request" as const,
      label: action,
      tooltip: action === "resume" ? "Saved state is corrupted" : undefined,
      params: { method: "session.recover", payload: { action } },
    })),
  };
  render(
    <StateProvider
      initialState={
        {
          agentProfiles: { items: [{ id: "profile" }] },
          taskSessions: { items: { session } },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{ sessionId: "session", kind: "generic", metadata }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  expect(screen.getByTestId(FRESH_BUTTON).getAttribute("data-recommended")).toBe("true");
  expect(screen.getByTestId(RESUME_BUTTON).getAttribute("title")).toBe("Saved state is corrupted");
});
it("keeps remediation available when a healthy session has no recovery owner", () => {
  const healthy = {
    ...session,
    state: "WAITING_FOR_INPUT" as const,
    error_message: "",
    metadata: {},
  };
  const row = message("provider_quota_limited");
  row.metadata = { ...row.metadata, remediation_url: "https://opencode.ai/workspace/demo/go" };
  render(
    <StateProvider
      initialState={
        { taskSessions: { items: { session: healthy } } } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryProvider session={healthy} messages={[row]} taskId="task" enabled>
        <ActionMessage comment={row} />
        <Owner />
      </SessionRecoveryProvider>
    </StateProvider>,
  );
  expect(screen.queryByTestId(RECOVERY_CARD)).toBeNull();
  expect(screen.getByTestId("remediation-link")).toBeTruthy();
});

it("redacts guard summaries after a failed restore", () => {
  render(
    <StateProvider>
      <SessionRecoveryCard
        model={{ sessionId: "session", kind: "generic" }}
        actions={
          {
            ...actions,
            guardDetails: { retryable: true },
            recoveryError: new Error("Restore failed: token=summary-secret-fixture"),
          } as SessionRecoveryActions
        }
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  expect(document.body.textContent).not.toContain("summary-secret-fixture");
});

it("withholds restore during a retryable guard in the composer", () => {
  render(
    <StateProvider>
      <SessionRecoveryCard
        model={{ sessionId: "session", kind: "generic" }}
        actions={{
          ...actions,
          guardDetails: { kind: "session_recovery_in_progress", retryable: true },
          recoveryError: new Error("busy"),
        }}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  expect(screen.queryByTestId(RESTORE_WORKSPACE_BUTTON)).toBeNull();
  expect(screen.getByTestId(RESUME_BUTTON)).toBeTruthy();
});
