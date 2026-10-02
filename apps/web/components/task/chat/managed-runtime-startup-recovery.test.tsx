import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { ActiveSessionRecovery } from "@/lib/active-session-recovery";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { Message } from "@/lib/types/http";
import { SessionRecoveryCard } from "./session-recovery-card";
import { ScriptExecutionMessage } from "./messages/script-execution-message";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const handleRecover = vi.fn();
const actions = {
  busyAction: null,
  recoveryError: null,
  guardDetails: null,
  branchDetails: null,
  recoveryNotice: null,
  manualRecoveryFailure: null,
  managedCloneRecoveryStamp: null,
  handleRecover,
  handleRestore: vi.fn(),
  handleNewBranch: vi.fn(),
} as unknown as SessionRecoveryActions;

function renderRecovery(reason: string, attempts: number) {
  const model: ActiveSessionRecovery = {
    sessionId: "session-1",
    kind: "managed_runtime_startup",
    summary: "managed runtime startup failed",
    details: "reason=hidden-diagnostic attempts=2",
    metadata: {
      failure_kind: "managed_runtime_startup",
      startup_reason: reason,
      startup_attempts: attempts,
      startup_npm_code: "ECONNRESET",
      error_output: "reason=hidden-diagnostic attempts=2",
    } as never,
  };

  render(
    <StateProvider
      initialState={
        {
          taskSessions: {
            items: { "session-1": { agent_profile_id: "profile-1" } },
          },
          agentProfiles: { items: [{ id: "profile-1" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard model={model} actions={actions} onNewSession={vi.fn()} />
    </StateProvider>,
  );
}

it.each([
  {
    reason: "npm_transient",
    attempts: 2,
    title: "npm could not prepare the runtime",
    summary:
      "Kandev retried the same runtime once after a temporary npm failure (2 attempts total).",
  },
  {
    reason: "early_exit",
    attempts: 2,
    title: "Agent stopped during startup",
    summary: "The agent process exited before initialization. Startup was attempted 2 times.",
  },
  {
    reason: "cleanup_failed",
    attempts: 1,
    title: "Agent process could not be stopped",
    summary:
      "Kandev could not confirm that the first process had stopped. It did not start a replacement.",
  },
])(
  "shows accurate $reason recovery without exposing diagnostic prose",
  ({ reason, attempts, title, summary }) => {
    renderRecovery(reason, attempts);

    const card = screen.getByTestId("session-recovery-card");
    expect(screen.getByRole("heading", { name: title })).toBeTruthy();
    expect(card.textContent).toContain(summary);
    expect(screen.getAllByTestId("managed-runtime-npm-retry-button")).toHaveLength(1);
    expect(card.querySelector("details")?.open).toBe(false);

    fireEvent.click(screen.getByTestId("managed-runtime-npm-retry-button"));
    expect(handleRecover).toHaveBeenCalledWith("runtime_retry");
  },
);

it("shows retry progress in the existing agent boot message", () => {
  const message = {
    id: "boot-1",
    task_id: "task-1",
    session_id: "session-1",
    author_type: "agent",
    content: "",
    type: "script_execution",
    created_at: "2026-10-02T10:00:00Z",
    metadata: {
      script_type: "agent_boot",
      agent_name: "Claude",
      command: "npx claude-agent-acp",
      status: "running",
      startup_retrying: true,
      startup_retry_attempt: 2,
      startup_retry_max_attempts: 2,
    },
  } as unknown as Message;

  render(<ScriptExecutionMessage comment={message} />);

  expect(screen.getByText("Retrying agent startup (attempt 2 of 2)")).toBeTruthy();
});
