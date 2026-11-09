import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import type { PermissionPresentation } from "@/lib/types/permission";
import { PermissionRequestMessage } from "./permission-request-message";
import { ToolCallMessage } from "./tool-call-message";
import { KandevToolMessage } from "./kandev-tool-message";
import { kandevToolCall } from "./kandev-tool-message.test-helpers";

function permissionMessage(
  presentation: PermissionPresentation | undefined,
  status?: "pending" | "approved",
  toolCallId = "tool-1",
): Message {
  return {
    id: "message-permission",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "Run git status",
    type: "permission_request",
    created_at: "2026-10-29T12:00:00Z",
    metadata: {
      request_id: "request-1",
      pending_id: "pending-1",
      tool_call_id: toolCallId,
      action_type: "command",
      action_details: { command: "git status" },
      status,
      ...(presentation ? { permission_presentation: presentation } : {}),
      options: [{ option_id: "allow-once", name: "Allow once", kind: "allow_once" }],
    },
  };
}

const PERMISSION_MESSAGE_TEST_ID = "permission-request-message";
const PERMISSION_ACTION_ROW_TEST_ID = "permission-action-row";
const PERMISSION_APPROVE_TEST_ID = "permission-approve";

describe("PermissionRequestMessage presentation", () => {
  afterEach(cleanup);

  it("renders no input prompt for an automatic pending row", () => {
    render(<PermissionRequestMessage comment={permissionMessage("automatic")} />);

    expect(screen.queryByTestId(PERMISSION_MESSAGE_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(PERMISSION_ACTION_ROW_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(PERMISSION_APPROVE_TEST_ID)).toBeNull();
  });

  it("renders non-actionable feedback for an unavailable row", () => {
    render(<PermissionRequestMessage comment={permissionMessage("unavailable")} />);

    const message = screen.getByTestId(PERMISSION_MESSAGE_TEST_ID);
    expect(message.getAttribute("data-permission-presentation")).toBe("unavailable");
    expect(message.textContent).toContain("Expired");
    expect(screen.queryByTestId(PERMISSION_ACTION_ROW_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(PERMISSION_APPROVE_TEST_ID)).toBeNull();
  });

  it("keeps the interactive prompt reachable for a manual row", () => {
    render(<PermissionRequestMessage comment={permissionMessage("interactive")} />);

    const message = screen.getByTestId(PERMISSION_MESSAGE_TEST_ID);
    expect(message.getAttribute("data-permission-presentation")).toBe("interactive");
    expect(message.textContent).toContain("Pending Approval");
    expect(screen.getByTestId(PERMISSION_ACTION_ROW_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(PERMISSION_APPROVE_TEST_ID)).toBeTruthy();
  });

  it("treats an unmarked legacy row as interactive", () => {
    render(<PermissionRequestMessage comment={permissionMessage(undefined)} />);

    expect(screen.getByTestId(PERMISSION_ACTION_ROW_TEST_ID)).toBeTruthy();
  });
});

describe("attached unavailable permission feedback", () => {
  afterEach(cleanup);

  it("renders non-actionable feedback on a generic tool row", () => {
    const toolCall: Message = {
      id: "tool-message",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      author_type: "agent",
      content: "Run git status",
      type: "tool_call",
      created_at: "2026-10-29T12:00:00Z",
      metadata: { tool_call_id: "tool-1", title: "Run git status", status: "complete" },
    };

    render(
      <ToolCallMessage comment={toolCall} permissionMessage={permissionMessage("unavailable")} />,
    );

    expect(screen.getByTestId("permission-unavailable-feedback")).toBeTruthy();
    expect(screen.queryByTestId(PERMISSION_ACTION_ROW_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(PERMISSION_APPROVE_TEST_ID)).toBeNull();
  });

  it("renders non-actionable feedback on a registered Kandev tool row", () => {
    render(
      <KandevToolMessage
        comment={kandevToolCall({ toolName: "mcp__kandev__list_tasks_kandev" })}
        permissionMessage={permissionMessage("unavailable", undefined, "tc-1")}
      />,
    );

    expect(screen.getByTestId("permission-unavailable-feedback")).toBeTruthy();
    expect(screen.queryByTestId(PERMISSION_ACTION_ROW_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("kandev-tool-permission")).toBeNull();
  });
});
