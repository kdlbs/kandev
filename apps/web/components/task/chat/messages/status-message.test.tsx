import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { StatusMessage } from "./status-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

afterEach(cleanup);

function modelSelectionWarningMessage(): Message {
  return {
    id: "status-1",
    session_id: toSessionId("session-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    content: "",
    type: "status",
    created_at: "2026-08-15T00:00:00Z",
    metadata: {
      variant: "warning",
      kind: "model_selection_warning",
      reason: "requested_not_advertised",
      requested_model: "claude-sonnet-4",
      effective_model: "claude-haiku-4",
      agent_id: "claude-acp",
      executor_type: "ssh",
      executor_profile_id: "executor-1",
      remediation: ["executor_credentials", "copied_agent_configuration", "agent_version"],
    },
  };
}

describe("StatusMessage model selection warnings", () => {
  it("renders the persisted decision context and remediation guidance", () => {
    render(<StatusMessage comment={modelSelectionWarningMessage()} />);

    expect(screen.getByText("The executor could not use the saved model selection.")).toBeTruthy();
    expect(screen.getByText("claude-sonnet-4")).toBeTruthy();
    expect(screen.getByText("claude-haiku-4")).toBeTruthy();
    expect(screen.getByText("claude-acp")).toBeTruthy();
    expect(screen.getByText("executor-1")).toBeTruthy();
    expect(screen.getByText("Check executor credentials.")).toBeTruthy();
    expect(screen.getByText("Check the copied agent configuration.")).toBeTruthy();
    expect(screen.getByText("Check the agent version in the executor.")).toBeTruthy();
    expect(screen.getByText("The saved model was not advertised by the executor.")).toBeTruthy();
  });
});

describe("StatusMessage branch replacement warnings", () => {
  it("states that conversation history continued but lost code did not", () => {
    const comment: Message = {
      id: "status-branch-1",
      session_id: toSessionId("session-1"),
      task_id: toTaskId("task-1"),
      author_type: "agent",
      content: "branch_recreated",
      type: "status",
      created_at: "2026-08-15T00:00:00Z",
      metadata: {
        variant: "warning",
        kind: "branch_recreated",
        original_branch: "feature/lost",
        new_branch: "kandev/task-recovery-1",
        base_branch: "main",
      },
    };

    render(<StatusMessage comment={comment} />);

    expect(screen.getByTestId("branch-recreated-warning")).toBeTruthy();
    expect(screen.getByText("feature/lost")).toBeTruthy();
    expect(screen.getByText("kandev/task-recovery-1")).toBeTruthy();
    expect(screen.getByText("main")).toBeTruthy();
    expect(screen.getByText(/conversation history continues/i)).toBeTruthy();
    expect(screen.getByText(/code changes.*not recovered/i)).toBeTruthy();
  });
});
