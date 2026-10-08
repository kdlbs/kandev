import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { ScriptExecutionMessage } from "./script-execution-message";

afterEach(cleanup);

function workflowScriptMessage(metadata: Record<string, unknown> = {}): Message {
  return {
    id: "workflow-script-message",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "all checks passed",
    type: "script_execution",
    created_at: "2026-09-05T10:00:00Z",
    metadata: {
      script_type: "workflow_step",
      workflow_step_name: "Review",
      trigger: "on_exit",
      command: "npm test",
      status: "exited",
      exit_code: 0,
      output_truncated: true,
      ...metadata,
    },
  } as Message;
}

describe("ScriptExecutionMessage workflow scripts", () => {
  it("shows lifecycle context, command output, and truncation state", () => {
    render(<ScriptExecutionMessage comment={workflowScriptMessage()} />);
    fireEvent.click(screen.getByText("Review · On Exit"));

    expect(screen.getByText("Run script")).toBeTruthy();
    expect(screen.getAllByText("npm test").length).toBe(2);
    expect(screen.getByText("all checks passed")).toBeTruthy();
    expect(screen.getByText("Output was truncated after reaching the safety limit.")).toBeTruthy();
  });

  it("keeps a failed workflow script visible with its reason", () => {
    render(
      <ScriptExecutionMessage
        comment={workflowScriptMessage({
          status: "failed",
          exit_code: 1,
          error: "command failed",
          output_truncated: false,
        })}
      />,
    );
    fireEvent.click(screen.getByText("Review · On Exit"));

    expect(screen.getByText("command failed")).toBeTruthy();
    expect(screen.getByText("Exit code: 1")).toBeTruthy();
  });
});
