import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import { ToolExecuteMessage } from "./tool-execute-message";

afterEach(cleanup);

type ShellOutput = {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
  truncated?: boolean;
};

function executeMessage(
  status: "pending" | "running" | "in_progress" | "complete" | "error" | "cancelled",
  output?: ShellOutput,
): Message {
  return {
    id: `message-${status}`,
    session_id: sessionId("session-1"),
    task_id: taskId("task-1"),
    author_type: "agent",
    content: "printf command-output",
    type: "tool_execute",
    created_at: "2026-07-14T12:00:00Z",
    metadata: {
      status,
      normalized: {
        shell_exec: {
          command: "printf command-output",
          work_dir: "/workspace",
          output,
        },
      },
    },
  };
}

function expandCommand() {
  fireEvent.click(screen.getAllByText("printf command-output")[0]);
}

describe("ToolExecuteMessage command result", () => {
  it("shows output and an exact successful exit code", () => {
    render(
      <ToolExecuteMessage
        comment={executeMessage("complete", { exit_code: 0, stdout: "command output\n" })}
      />,
    );

    expect(screen.getByLabelText("Command succeeded")).toBeTruthy();
    expandCommand();
    expect(screen.getByText("command output")).toBeTruthy();
    expect(screen.getByText("Exit code 0")).toBeTruthy();
  });

  it("shows a known nonzero exit as failure", () => {
    render(
      <ToolExecuteMessage
        comment={executeMessage("complete", { exit_code: 7, stderr: "failed output\n" })}
      />,
    );

    expect(screen.getByLabelText("Command failed")).toBeTruthy();
    expandCommand();
    expect(screen.getByText("failed output")).toBeTruthy();
    expect(screen.getByText("Exit code 7")).toBeTruthy();
  });

  it("keeps an unavailable exit code neutral", () => {
    render(
      <ToolExecuteMessage
        comment={executeMessage("complete", { stdout: "result without status" })}
      />,
    );

    expect(screen.queryByLabelText("Command succeeded")).toBeNull();
    expect(screen.queryByLabelText("Command failed")).toBeNull();
    expandCommand();
    expect(screen.getByText("Exit code unavailable")).toBeTruthy();
  });

  it("auto-expands live output without a terminal exit label", () => {
    render(
      <ToolExecuteMessage comment={executeMessage("running", { stdout: "partial output\n" })} />,
    );

    expect(screen.getByLabelText("Command running")).toBeTruthy();
    expect(screen.getByText("partial output")).toBeTruthy();
    expect(screen.queryByText(/Exit code/)).toBeNull();
  });

  it("treats ACP in_progress output as live", () => {
    render(
      <ToolExecuteMessage
        comment={executeMessage("in_progress", { stdout: "ACP partial output\n" })}
      />,
    );

    expect(screen.getByLabelText("Command running")).toBeTruthy();
    expect(screen.getByText("ACP partial output")).toBeTruthy();
    expect(screen.queryByText(/Exit code/)).toBeNull();
  });

  it("shows truncation and unknown exit state together", () => {
    render(
      <ToolExecuteMessage
        comment={executeMessage("complete", { stdout: "latest output", truncated: true })}
      />,
    );

    expandCommand();
    expect(screen.getByText("Output truncated")).toBeTruthy();
    expect(screen.getByText("Exit code unavailable")).toBeTruthy();
  });

  it("shows terminal details for a cancelled command", () => {
    render(
      <ToolExecuteMessage comment={executeMessage("cancelled", { stdout: "cancelled output" })} />,
    );

    expandCommand();
    expect(screen.getByText("cancelled output")).toBeTruthy();
    expect(screen.getByText("Exit code unavailable")).toBeTruthy();
  });
});
