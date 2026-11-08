import { claimSessionRecovery } from "@/hooks/domains/session/session-recovery-pending";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
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
const RUNTIME_RETRY_BUTTON = "managed-runtime-npm-retry-button";
const MISE_SUMMARY = "mise did not trust this workspace's configuration.";
const EXIT_CODE_ONE = "Exit code: 1";
const PREREQUISITE = "session-recovery-prerequisite";
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
    expect(screen.getByTestId("session-recovery-history").textContent).toContain(CONNECTION_LOST);
    expect(screen.getByTestId("session-recovery-history-details")).toBeTruthy();
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
      fireEvent.click(screen.getByTestId(RUNTIME_RETRY_BUTTON));
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
      expect(screen.getByTestId(RUNTIME_RETRY_BUTTON)).toHaveProperty("disabled", true);
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
    expect(screen.getByTestId(RUNTIME_RETRY_BUTTON)).toBeTruthy();
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

function processExitModel(reason: string, exitCode: number): ActiveSessionRecovery {
  return {
    sessionId: "session",
    stamp: "failure",
    kind: "managed_runtime_startup",
    error: {
      message: "The agent exited before startup completed.",
      occurredAt: "2026-11-07T12:00:00Z",
      phase: "bootstrap",
      attemptId: "resume-1",
      executionId: "exec-1",
      causes: [
        {
          operation: "start",
          code: "process_exited",
          reason,
          exit_code: exitCode,
        },
      ],
    },
  };
}

function renderProcessExitCard(
  reason: string,
  exitCode: number,
  extraActions: Partial<SessionRecoveryActions> = {},
) {
  return render(
    <StateProvider
      initialState={
        {
          agentProfiles: { items: [{ id: "profile" }] },
          taskSessions: { items: { session } },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={processExitModel(reason, exitCode)}
        actions={{ ...actions, ...extraActions } as SessionRecoveryActions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
}

it("renders the final process exit for a managed runtime retry failure", () => {
  renderProcessExitCard("mise_config_untrusted", 1);
  const card = screen.getByTestId(RECOVERY_CARD);
  expect(screen.getByRole("heading", { name: "Agent startup blocked by mise" })).toBeTruthy();
  expect(card.textContent).toContain(MISE_SUMMARY);
  expect(screen.getByTestId(PREREQUISITE)).toBeTruthy();
  expect(card.textContent).toContain(EXIT_CODE_ONE);
  expect(screen.getByTestId(RUNTIME_RETRY_BUTTON)).toBeTruthy();
  expect(screen.queryByTestId(RESUME_BUTTON)).toBeNull();
});

it("renders an unknown final process exit without mise attribution", () => {
  renderProcessExitCard("unclassified", 3);
  const card = screen.getByTestId(RECOVERY_CARD);
  expect(screen.getByRole("heading", { name: "Agent exited before startup" })).toBeTruthy();
  expect(card.textContent).toContain("The agent exited before startup completed.");
  expect(screen.queryByTestId(PREREQUISITE)).toBeNull();
  expect(card.textContent).toContain("Exit code: 3");
});

it("keeps a later branch refusal visible over the earlier process cause", () => {
  renderProcessExitCard("mise_config_untrusted", 1, {
    branchDetails: {
      kind: "branch_unrecoverable",
      recovery_action: "resume_new_branch",
      original_branch: "feature/gone",
    } as never,
  });
  const card = screen.getByTestId(RECOVERY_CARD);
  expect(card.textContent).toContain("Branch is no longer available");
  expect(card.textContent).not.toContain(MISE_SUMMARY);
  expect(screen.queryByTestId(PREREQUISITE)).toBeNull();
  expect(card.textContent).toContain(EXIT_CODE_ONE);
});

it("keeps a later guard refusal visible over the earlier process cause", () => {
  renderProcessExitCard("mise_config_untrusted", 1, {
    guardDetails: { kind: "session_recovery_in_progress", retryable: true } as never,
    recoveryError: new Error("Recovery refused for a different reason"),
  });
  const card = screen.getByTestId(RECOVERY_CARD);
  expect(card.textContent).toContain("Recovery refused for a different reason");
  expect(card.textContent).not.toContain(MISE_SUMMARY);
  expect(screen.queryByTestId(PREREQUISITE)).toBeNull();
  expect(card.textContent).toContain(EXIT_CODE_ONE);
});

it("keeps a later manual recovery failure visible over the earlier process cause", () => {
  renderProcessExitCard("mise_config_untrusted", 1, {
    manualRecoveryFailure: {
      operation: "resume",
      sessionId: "session",
      errorStamp: "failure",
      requestKey: "request",
      operationId: 1,
    },
    recoveryError: new Error("A different resume failure"),
  });
  const card = screen.getByTestId(RECOVERY_CARD);
  expect(card.textContent).toContain("Failed to resume session");
  expect(card.textContent).not.toContain("mise did not trust this workspace's configuration.");
  expect(screen.queryByTestId(PREREQUISITE)).toBeNull();
  expect(card.textContent).toContain(EXIT_CODE_ONE);
});
