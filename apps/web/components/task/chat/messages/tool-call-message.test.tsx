import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ToolCallMessage } from "./tool-call-message";

afterEach(cleanup);

function controlMessage(title: string, control?: string): Message {
  return {
    id: `tool-${title}`,
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: title,
    type: "tool_call",
    created_at: "2026-11-01T12:00:00Z",
    metadata: {
      title,
      status: "complete",
      normalized: {
        kind: "generic",
        generic: { name: "other", subagent_control: control },
      },
    },
  };
}

// @covers AC-AGENTS-SUBAGENT-LIFECYCLE-004.2
describe("ToolCallMessage subagent control labels", () => {
  it.each([
    ["wait", "Waited for subagents"],
    ["close", "Closed subagent"],
    ["message", "Messaged subagent"],
    ["read", "Read subagent"],
    ["list", "Listed subagents"],
    ["resume", "Resumed subagent"],
  ])("renders the localized label for %s", (control, label) => {
    render(<ToolCallMessage comment={controlMessage("provider operation", control)} />);

    expect(screen.getByText(label)).toBeTruthy();
  });

  it("falls back to the provider title for an unknown value", () => {
    render(<ToolCallMessage comment={controlMessage("Custom Operation", "future-control")} />);

    expect(screen.getByText("Custom Operation")).toBeTruthy();
  });

  it("keeps a generic wait title when there is no recognized provider control", () => {
    render(<ToolCallMessage comment={controlMessage("wait")} />);

    expect(screen.getByText("wait")).toBeTruthy();
  });
});
