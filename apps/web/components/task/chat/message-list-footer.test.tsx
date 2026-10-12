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

import { MessageListFooter } from "./message-list-footer";

const AGENT_STATUS_TEST_ID = "agent-status";
const ACTION_MESSAGE_TEST_ID = "action-message";
const SESSION_ID = "session-1";
const FAILED_SESSION_STATE = "FAILED" as const;

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

// eslint-disable-next-line max-lines-per-function -- footer ownership cases share one focused fixture.
describe("MessageListFooter", () => {
  it("lets an actionable footer failure own the failure presentation", () => {
    render(
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId(ACTION_MESSAGE_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(AGENT_STATUS_TEST_ID)).toBeNull();
  });

  it("keeps the generic status for a failed session without an actionable footer", () => {
    render(
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[]}
      />,
    );

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
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[]}
      />,
    );
    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();

    rerender(
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.queryByTestId(AGENT_STATUS_TEST_ID)).toBeNull();
    expect(screen.getByTestId(ACTION_MESSAGE_TEST_ID)).toBeTruthy();
  });

  it("does not restore stale missing-branch recovery after a later failure", () => {
    render(
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[actionableFailure, laterFailure]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(ACTION_MESSAGE_TEST_ID)).toBeNull();
  });

  it("hides launch failure footer surfaces when the task card owns the error", () => {
    render(
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[actionableFailure]}
        launchErrorOwned
      />,
    );

    expect(screen.queryByTestId(AGENT_STATUS_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(ACTION_MESSAGE_TEST_ID)).toBeNull();
  });

  it("keeps stale launch failure footer history when the current card owns the error", () => {
    const staleFailure = {
      ...actionableFailure,
      id: "failure-stale",
      content: "Stale branch recovery",
      metadata: {
        ...actionableFailure.metadata,
        launch_error_stamp: "old-stamp",
      },
    } satisfies Message;
    const currentFailure = {
      ...actionableFailure,
      id: "failure-current",
      content: "Current branch recovery",
      metadata: {
        ...actionableFailure.metadata,
        launch_error_stamp: "current-stamp",
      },
    } satisfies Message;

    render(
      <MessageListFooter
        sessionState={FAILED_SESSION_STATE}
        sessionId={SESSION_ID}
        messages={[]}
        footerActionMessages={[staleFailure, currentFailure]}
        launchErrorOwned
        launchErrorStamp="current-stamp"
      />,
    );

    expect(screen.getByText("Stale branch recovery")).toBeTruthy();
    expect(screen.queryByText("Current branch recovery")).toBeNull();
  });
});
