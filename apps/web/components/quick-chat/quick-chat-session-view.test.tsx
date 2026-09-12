import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const sessionRows = vi.hoisted(
  () =>
    ({}) as Record<
      string,
      {
        task_id?: string;
        agent_profile_id?: string;
        is_passthrough?: boolean;
        metadata?: Record<string, unknown> | null;
      }
    >,
);
const quickChatSessions = vi.hoisted(
  () => [] as Array<{ sessionId: string; taskId?: string; agentProfileId?: string }>,
);
const useEnsureTaskSession = vi.hoisted(() => vi.fn());
const useTask = vi.hoisted(() => vi.fn());
const useSessionResumption = vi.hoisted(() =>
  vi.fn(() => ({
    resumptionState: "idle",
    sessionStatus: null,
    error: null,
    notice: null,
    recoveryFailure: null,
    taskSessionState: null,
    worktreePath: null,
    worktreeBranch: null,
    resumeSession: vi.fn(),
  })),
);
const useTaskStatusSummary = vi.hoisted(() => vi.fn());

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      taskSessions: { items: sessionRows },
      quickChat: { sessions: quickChatSessions },
      agentProfiles: { items: [] },
    }),
}));

vi.mock("@/hooks/use-ensure-task-session", () => ({ useEnsureTaskSession }));
vi.mock("@/hooks/use-task", () => ({ useTask }));
vi.mock("@/hooks/domains/session/use-session-resumption", () => ({ useSessionResumption }));
vi.mock("@/hooks/domains/task/use-task-status-summary", () => ({ useTaskStatusSummary }));
vi.mock("@/components/task/passthrough-terminal", () => ({
  PassthroughTerminal: () => <div data-testid="passthrough-terminal" />,
}));
vi.mock("./quick-chat-content", () => ({
  QuickChatContent: () => <div data-testid="quick-chat-content" />,
}));
vi.mock("@/components/task/chat/session-bootstrap-recovery-card", () => ({
  SessionBootstrapRecoveryCard: ({
    error,
    automaticRecovery,
  }: {
    error: { stamp: string };
    automaticRecovery?: {
      notice: string | null;
      recoveryFailure: { outcome: string } | null;
    };
  }) => (
    <div data-testid="session-bootstrap-recovery-card">
      {error.stamp}
      {automaticRecovery?.notice ?? ""}
      {automaticRecovery?.recoveryFailure?.outcome ?? ""}
    </div>
  ),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { QuickChatSessionView } from "./quick-chat-session-view";

const DESCRIPTOR_TASK_ID = "task-from-descriptor";
const HYDRATED_TASK_ID = "task-from-hydrated-row";
const WORKSPACE_ID = "workspace-1";
const BOOTSTRAP_OCCURRED_AT = "2026-09-11T10:00:00Z";
const BOOTSTRAP_PREVIEW = "The agent could not start.";
const WORKSPACE_READ_ONLY_NOTICE = "workspace restored read-only";
const RESUME_FAILURE = "resume failed";
const RECOVERY_CARD_TEST_ID = "session-bootstrap-recovery-card";

const session = {
  kind: "chat" as const,
  sessionId: "session-1",
  workspaceId: WORKSPACE_ID,
};

afterEach(() => {
  cleanup();
  delete sessionRows[session.sessionId];
  quickChatSessions.length = 0;
  vi.clearAllMocks();
  useTask.mockReset();
  useTask.mockReturnValue(null);
  useTaskStatusSummary.mockReset();
  useTaskStatusSummary.mockReturnValue(undefined);
});

// @covers AC-TASKS-QUICK-CHAT-EXPIRATION-001.2
// eslint-disable-next-line max-lines-per-function -- session ownership and recovery outcomes share one view harness.
describe("QuickChatSessionView session resumption", () => {
  it("prefers the descriptor task id over a conflicting hydrated row", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };

    render(<QuickChatSessionView session={{ ...session, taskId: DESCRIPTOR_TASK_ID }} />);

    expect(useSessionResumption).toHaveBeenCalledWith(DESCRIPTOR_TASK_ID, session.sessionId, null);
  });

  it("waits for session hydration before resuming a descriptor task", () => {
    const view = render(
      <QuickChatSessionView session={{ ...session, taskId: DESCRIPTOR_TASK_ID }} />,
    );

    expect(useSessionResumption).toHaveBeenLastCalledWith(null, session.sessionId, null);

    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };
    view.rerender(<QuickChatSessionView session={{ ...session, taskId: DESCRIPTOR_TASK_ID }} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(
      DESCRIPTOR_TASK_ID,
      session.sessionId,
      null,
    );
  });

  it("falls back to the hydrated task-session row when the descriptor has no task id", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };

    render(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenCalledWith(HYDRATED_TASK_ID, session.sessionId, null);
  });

  it("passes the hydrated task archive state to session recovery", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };
    useTask.mockReturnValue({ isArchived: true });

    render(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenCalledWith(HYDRATED_TASK_ID, session.sessionId, true);
  });

  it("uses hydrated Quick Chat ownership when the ephemeral task is absent from kanban tasks", () => {
    const view = render(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(null, session.sessionId, null);

    quickChatSessions.push({ sessionId: session.sessionId, taskId: HYDRATED_TASK_ID });
    view.rerender(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(
      HYDRATED_TASK_ID,
      session.sessionId,
      false,
    );
  });

  it("passes a null task id until session hydration provides one", () => {
    const view = render(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(null, session.sessionId, null);

    sessionRows[session.sessionId] = { task_id: "task-after-hydration" };
    view.rerender(<QuickChatSessionView session={session} />);

    expect(useSessionResumption).toHaveBeenLastCalledWith(
      "task-after-hydration",
      session.sessionId,
      null,
    );
  });

  it("gives a matching bootstrap failure to the inline recovery card", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };
    useTask.mockReturnValue({ isArchived: false, workspaceId: WORKSPACE_ID });
    useTaskStatusSummary.mockReturnValue({
      active_error: {
        session_id: session.sessionId,
        stamp: "bootstrap-1",
        occurred_at: BOOTSTRAP_OCCURRED_AT,
        preview: BOOTSTRAP_PREVIEW,
        phase: "bootstrap",
      },
    });

    render(<QuickChatSessionView session={session} />);

    expect(screen.getByTestId(RECOVERY_CARD_TEST_ID).textContent).toBe("bootstrap-1");
    expect(screen.getByTestId("quick-chat-content")).toBeTruthy();
  });

  it("passes automatic read-only recovery into the inline card", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };
    useTask.mockReturnValue({ isArchived: false, workspaceId: WORKSPACE_ID });
    useTaskStatusSummary.mockReturnValue({
      active_error: {
        session_id: session.sessionId,
        stamp: "bootstrap-read-only",
        occurred_at: BOOTSTRAP_OCCURRED_AT,
        preview: BOOTSTRAP_PREVIEW,
        phase: "bootstrap",
      },
    });
    useSessionResumption.mockReturnValue({
      resumptionState: "resumed",
      sessionStatus: null,
      error: null,
      notice: WORKSPACE_READ_ONLY_NOTICE,
      recoveryFailure: { outcome: "workspace_read_only", resumeError: RESUME_FAILURE },
      taskSessionState: "FAILED",
      worktreePath: null,
      worktreeBranch: null,
      resumeSession: vi.fn(),
    } as never);

    render(<QuickChatSessionView session={session} />);

    expect(screen.getByTestId(RECOVERY_CARD_TEST_ID).textContent).toContain(
      WORKSPACE_READ_ONLY_NOTICE,
    );
    expect(screen.getByTestId(RECOVERY_CARD_TEST_ID).textContent).not.toContain(RESUME_FAILURE);
  });

  it("passes automatic recovery failure into the inline card", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID };
    useTask.mockReturnValue({ isArchived: false, workspaceId: WORKSPACE_ID });
    useTaskStatusSummary.mockReturnValue({
      active_error: {
        session_id: session.sessionId,
        stamp: "bootstrap-failed",
        occurred_at: BOOTSTRAP_OCCURRED_AT,
        preview: BOOTSTRAP_PREVIEW,
        phase: "bootstrap",
      },
    });
    useSessionResumption.mockReturnValue({
      resumptionState: "error",
      sessionStatus: null,
      error: "Session recovery failed",
      notice: null,
      recoveryFailure: {
        outcome: "recovery_failed",
        resumeError: "raw resume failure",
        restoreError: "raw restore failure",
      },
      taskSessionState: "FAILED",
      worktreePath: null,
      worktreeBranch: null,
      resumeSession: vi.fn(),
    } as never);

    render(<QuickChatSessionView session={session} />);

    expect(screen.getByTestId(RECOVERY_CARD_TEST_ID).textContent).toContain("recovery_failed");
  });

  it("keeps the bootstrap recovery card and automatic outcome in passthrough Quick Chat", () => {
    sessionRows[session.sessionId] = { task_id: HYDRATED_TASK_ID, is_passthrough: true };
    useTask.mockReturnValue({ isArchived: false, workspaceId: WORKSPACE_ID });
    useTaskStatusSummary.mockReturnValue({
      active_error: {
        session_id: session.sessionId,
        stamp: "bootstrap-passthrough",
        occurred_at: BOOTSTRAP_OCCURRED_AT,
        preview: BOOTSTRAP_PREVIEW,
        phase: "bootstrap",
      },
    });
    useSessionResumption.mockReturnValue({
      resumptionState: "resumed",
      sessionStatus: null,
      error: null,
      notice: WORKSPACE_READ_ONLY_NOTICE,
      recoveryFailure: { outcome: "workspace_read_only", resumeError: "raw resume failure" },
      taskSessionState: "FAILED",
      worktreePath: null,
      worktreeBranch: null,
      resumeSession: vi.fn(),
    } as never);

    render(<QuickChatSessionView session={session} />);

    expect(screen.getByTestId(RECOVERY_CARD_TEST_ID).textContent).toContain(
      WORKSPACE_READ_ONLY_NOTICE,
    );
    expect(screen.getByTestId("passthrough-terminal")).toBeTruthy();
    expect(screen.queryByTestId("quick-chat-content")).toBeNull();
  });
});
