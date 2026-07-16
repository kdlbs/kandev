import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { sessionId, taskId, type Message } from "@/lib/types/http";
import type { UseShellCommandOutputResult } from "@/hooks/domains/session/use-shell-command-output";
import { ToolExecuteMessage } from "./tool-execute-message";

const useShellOutputMock = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/domains/session/use-shell-command-output", () => ({
  useShellCommandOutput: useShellOutputMock,
}));

afterEach(cleanup);

beforeEach(() => {
  useShellOutputMock.mockReset();
  useShellOutputMock.mockReturnValue(hookResult());
});

type ShellOutputSummary = {
  exit_code?: number;
  has_output?: boolean;
  stdout_bytes?: number;
  stderr_bytes?: number;
  truncated?: boolean;
  stdout?: string;
};

function executeMessage(
  status: "pending" | "running" | "in_progress" | "complete" | "error" | "cancelled",
  output: ShellOutputSummary = {},
  command = "printf normalized-command",
): Message {
  return {
    id: `message-${status}`,
    session_id: sessionId("session-1"),
    task_id: taskId("task-1"),
    author_type: "agent",
    content: "fallback message content",
    type: "tool_execute",
    created_at: "2026-07-16T12:00:00Z",
    metadata: {
      status,
      normalized: {
        shell_exec: {
          command,
          work_dir: "/workspace/with/a/long/path",
          output,
        },
      },
    },
  };
}

function hookResult(
  overrides: Partial<UseShellCommandOutputResult> = {},
): UseShellCommandOutputResult {
  return {
    snapshot: null,
    isLoading: false,
    error: null,
    retry: vi.fn(),
    ...overrides,
  };
}

function openOutput() {
  fireEvent.click(screen.getByRole("button", { name: /show command output/i }));
}

describe("ToolExecuteMessage command row", () => {
  it("keeps the normalized command and working directory visible while output is collapsed", () => {
    render(
      <ToolExecuteMessage
        comment={executeMessage("running", {
          has_output: true,
          stdout_bytes: 2048,
          stderr_bytes: 0,
        })}
      />,
    );

    const command = screen.getByTestId("tool-execute-command");
    expect(command.textContent).toBe("printf normalized-command");
    expect(command.className).toContain("whitespace-pre-wrap");
    expect(screen.getByText("/workspace/with/a/long/path")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /show command output/i }).getAttribute("aria-expanded"),
    ).toBe("false");
    expect(screen.queryByTestId("tool-execute-output")).toBeNull();
    expect(useShellOutputMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ isOpen: false, sessionId: "session-1" }),
    );
  });

  it("falls back to message content when the normalized command is absent", () => {
    render(<ToolExecuteMessage comment={executeMessage("complete", {}, "")} />);

    expect(screen.getByTestId("tool-execute-command").textContent).toBe("fallback message content");
  });

  it("renders loading and empty snapshot states only after opening", () => {
    useShellOutputMock.mockReturnValue(hookResult({ isLoading: true }));
    const view = render(<ToolExecuteMessage comment={executeMessage("running")} />);

    expect(screen.queryByText("Loading command output...")).toBeNull();
    openOutput();
    expect(screen.getByText("Loading command output...")).toBeTruthy();

    useShellOutputMock.mockReturnValue(
      hookResult({
        snapshot: {
          message_id: "message-running",
          status: "running",
          updated_at: "2026-07-16T12:00:01Z",
          output: {},
        },
      }),
    );
    view.rerender(<ToolExecuteMessage comment={executeMessage("running")} />);
    expect(screen.getByText("No command output yet.")).toBeTruthy();
    expect(screen.queryByText(/Exit code/)).toBeNull();
  });
});

describe("ToolExecuteMessage output states", () => {
  it("renders stdout, stderr, truncation, and a known nonzero exit from the snapshot", () => {
    useShellOutputMock.mockReturnValue(
      hookResult({
        snapshot: {
          message_id: "message-complete",
          status: "failed",
          updated_at: "2026-07-16T12:00:02Z",
          output: {
            stdout: "standard output",
            stderr: "standard error",
            truncated: true,
            exit_code: 7,
          },
        },
      }),
    );
    render(<ToolExecuteMessage comment={executeMessage("complete", { exit_code: 7 })} />);

    openOutput();
    expect(screen.getByText("standard output")).toBeTruthy();
    expect(screen.getByText("standard error")).toBeTruthy();
    expect(screen.getByText("Output truncated")).toBeTruthy();
    expect(screen.getByText("Exit code 7").className).toContain("text-red");
  });

  it("renders unknown terminal exit neutrally and never reads a transcript body from summary metadata", () => {
    useShellOutputMock.mockReturnValue(
      hookResult({
        snapshot: {
          message_id: "message-cancelled",
          status: "cancelled",
          updated_at: "2026-07-16T12:00:02Z",
          output: { stdout: "snapshot transcript" },
        },
      }),
    );
    render(
      <ToolExecuteMessage
        comment={executeMessage("cancelled", {
          has_output: true,
          stdout_bytes: 999,
          stdout: "forbidden summary transcript",
        })}
      />,
    );

    openOutput();
    expect(screen.getByText("snapshot transcript")).toBeTruthy();
    expect(screen.queryByText("forbidden summary transcript")).toBeNull();
    expect(screen.getByText("Exit code unavailable").className).toContain("text-muted-foreground");
  });

  it("keeps the latest snapshot visible on an error and retries on command", () => {
    const retry = vi.fn();
    useShellOutputMock.mockReturnValue(
      hookResult({
        snapshot: {
          message_id: "message-running",
          status: "running",
          updated_at: "2026-07-16T12:00:02Z",
          output: { stdout: "retained transcript" },
        },
        error: new Error("network unavailable"),
        retry,
      }),
    );
    render(<ToolExecuteMessage comment={executeMessage("running")} />);

    openOutput();
    expect(screen.getByText("retained transcript")).toBeTruthy();
    expect(screen.getByText("Command output unavailable.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledOnce();
  });
});
