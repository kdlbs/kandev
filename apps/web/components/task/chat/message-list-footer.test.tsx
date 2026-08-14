import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

vi.mock("@/components/task/chat/messages/agent-status", () => ({
  AgentStatus: () => <div data-testid="agent-status">Agent failed</div>,
}));

vi.mock("@/components/task/chat/message-renderer", () => ({
  MessageRenderer: ({ comment }: { comment: Message }) => (
    <div data-testid="action-message">{comment.content}</div>
  ),
}));

let mockResumeSkippedSessionIds: Record<string, true> = {};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { tasks: { resumeSkippedSessionIds: Record<string, true> } }) => unknown,
  ) => selector({ tasks: { resumeSkippedSessionIds: mockResumeSkippedSessionIds } }),
}));

vi.mock("@/components/task/chat/session-resume-start-button", () => ({
  SessionResumeStartButton: () => <div data-testid={RESUME_BUTTON_TEST_ID}>Start agent</div>,
}));

import { MessageListFooter } from "./message-list-footer";

const AGENT_STATUS_TEST_ID = "agent-status";
const SESSION_ID = "session-1";
const RESUME_BUTTON_TEST_ID = "resume-start-button";

afterEach(cleanup);

const actionableFailure = {
  id: "failure-1",
  session_id: toSessionId(SESSION_ID),
  task_id: toTaskId("task-1"),
  author_type: "agent",
  type: "status",
  created_at: "2026-07-22T00:00:00Z",
  content: "Branch recovery",
  metadata: {
    variant: "warning",
    failure_kind: "missing_pr_branch",
    actions: [{ type: "archive_task", label: "Archive task" }],
  },
} satisfies Message;

const laterFailure = {
  ...actionableFailure,
  id: "failure-2",
  content: "Agent encountered an authentication error",
  metadata: {
    variant: "error",
    recovery_actions: true,
    actions: [{ type: "ws_request", label: "Resume session" }],
  },
} satisfies Message;

describe("MessageListFooter", () => {
  it("lets an actionable footer failure own the failure presentation", () => {
    render(
      <MessageListFooter
        sessionState="FAILED"
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId("action-message")).toBeTruthy();
    expect(screen.queryByTestId(AGENT_STATUS_TEST_ID)).toBeNull();
  });

  it("keeps the generic status for a failed session without an actionable footer", () => {
    render(<MessageListFooter sessionState="FAILED" sessionId={SESSION_ID} messages={[]} />);

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
  });

  it("retains the running status when an action message is hidden during startup", () => {
    render(
      <MessageListFooter
        sessionState="STARTING"
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
  });

  it("switches ownership when missing-branch recovery arrives after failure", () => {
    const { rerender } = render(
      <MessageListFooter sessionState="FAILED" sessionId={SESSION_ID} messages={[]} />,
    );
    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();

    rerender(
      <MessageListFooter
        sessionState="FAILED"
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.queryByTestId(AGENT_STATUS_TEST_ID)).toBeNull();
    expect(screen.getByTestId("action-message")).toBeTruthy();
  });

  it("does not restore stale missing-branch recovery after a later failure", () => {
    render(
      <MessageListFooter
        sessionState="FAILED"
        sessionId={SESSION_ID}
        messages={[actionableFailure, laterFailure]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("action-message")).toBeNull();
  });
});

const recoveryMessage = {
  id: "recovery-1",
  session_id: toSessionId("session-1"),
  task_id: toTaskId("task-1"),
  author_type: "agent",
  type: "status",
  created_at: "2026-07-22T00:00:00Z",
  content: "Agent process exited with code 1",
  metadata: {
    variant: "danger",
    recovery_actions: true,
    actions: [
      { type: "resume_session", label: "Resume session" },
      { type: "start_fresh", label: "Start fresh session" },
    ],
  },
} satisfies Message;

describe("MessageListFooter resume-skipped start button", () => {
  const withMessages = [actionableFailure];

  it("renders the Start agent button for a resume-skipped session with messages", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    render(
      <MessageListFooter
        sessionState="WAITING_FOR_INPUT"
        sessionId={SESSION_ID}
        messages={withMessages}
      />,
    );
    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();
  });

  it("hides the Start agent button for a resume-skipped FAILED session (recovery owns the affordance)", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    render(
      <MessageListFooter sessionState="FAILED" sessionId={SESSION_ID} messages={withMessages} />,
    );
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
  });

  it("renders the Start agent button for an empty resume-skipped session (empty description fallback)", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    render(
      <MessageListFooter sessionState="WAITING_FOR_INPUT" sessionId={SESSION_ID} messages={[]} />,
    );
    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();
  });

  it("hides the Start agent button when recovery actions are already visible", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    render(
      <MessageListFooter
        sessionState="WAITING_FOR_INPUT"
        sessionId={SESSION_ID}
        messages={[recoveryMessage]}
      />,
    );
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
  });

  it("renders the Start agent button when no recovery actions are visible", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    render(
      <MessageListFooter
        sessionState="WAITING_FOR_INPUT"
        sessionId={SESSION_ID}
        messages={withMessages}
      />,
    );
    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toBeTruthy();
  });

  it("hides the Start agent button for a resume-skipped RUNNING session", () => {
    mockResumeSkippedSessionIds = { [SESSION_ID]: true };
    render(
      <MessageListFooter sessionState="RUNNING" sessionId={SESSION_ID} messages={withMessages} />,
    );
    expect(screen.queryByTestId(RESUME_BUTTON_TEST_ID)).toBeNull();
  });
});
