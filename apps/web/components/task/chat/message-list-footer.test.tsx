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

afterEach(cleanup);

const AGENT_STATUS_TEST_ID = "agent-status";

const actionableFailure = {
  id: "failure-1",
  session_id: toSessionId("session-1"),
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
        sessionId="session-1"
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId("action-message")).toBeTruthy();
    expect(screen.queryByTestId(AGENT_STATUS_TEST_ID)).toBeNull();
  });

  it("keeps the generic status for a failed session without an actionable footer", () => {
    render(<MessageListFooter sessionState="FAILED" sessionId="session-1" messages={[]} />);

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
  });

  it("retains the running status when an action message is hidden during startup", () => {
    render(
      <MessageListFooter
        sessionState="STARTING"
        sessionId="session-1"
        messages={[]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
  });

  it("switches ownership when missing-branch recovery arrives after failure", () => {
    const { rerender } = render(
      <MessageListFooter sessionState="FAILED" sessionId="session-1" messages={[]} />,
    );
    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();

    rerender(
      <MessageListFooter
        sessionState="FAILED"
        sessionId="session-1"
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
        sessionId="session-1"
        messages={[actionableFailure, laterFailure]}
        footerActionMessages={[actionableFailure]}
      />,
    );

    expect(screen.getByTestId(AGENT_STATUS_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("action-message")).toBeNull();
  });
});
