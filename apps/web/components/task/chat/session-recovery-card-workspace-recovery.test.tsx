import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryProvider, useSessionComposerRecovery } from "./session-recovery-context";
import { SessionRecoveryCard } from "./session-recovery-card";

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
afterEach(cleanup);

const RESUME_BUTTON = "recovery-resume-button";
const FRESH_BUTTON = "recovery-fresh-button";
const RESTORE_WORKSPACE_BUTTON = "recovery-restore-workspace-button";
const RECOVERY_CARD = "session-recovery-card";
const relocate = vi.fn().mockResolvedValue(true);
const actions = {
  busyAction: null,
  recoveryError: null,
  guardDetails: null,
  branchDetails: null,
  recoveryNotice: null,
  managedCloneRecoveryStamp: null,
  handleRecover: vi.fn(),
  handleManagedCloneRelocation: relocate,
} as unknown as SessionRecoveryActions;
const session = {
  id: "session",
  task_id: "task",
  state: "FAILED",
  agent_profile_id: "profile",
  error_message: "Connection lost",
} as unknown as TaskSession;

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

describe("composer workspace recovery projection", () => {
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
