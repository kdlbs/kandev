import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MessageRenderer } from "./message-renderer";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

const fallbackOpenFile = vi.hoisted(() => vi.fn());
const useOpenFileAtLine = vi.hoisted(() =>
  vi.fn((onOpenFile: ((path: string) => void) | undefined) => (path: string) => onOpenFile?.(path)),
);

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({ openFile: fallbackOpenFile }),
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useOpenFileAtLine,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
}));

function message(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "message",
    content: "hello",
    created_at: "2026-06-16T12:00:00Z",
    ...overrides,
  };
}

describe("MessageRenderer markdown file links", () => {
  afterEach(() => {
    fallbackOpenFile.mockClear();
    useOpenFileAtLine.mockClear();
  });

  it("forwards the renderer session through edit file links", () => {
    const onOpenFile = vi.fn();

    render(
      <MessageRenderer
        comment={message({
          type: "tool_edit",
          content: "Edited /workspace/src/app.ts",
          metadata: {
            status: "complete",
            normalized: {
              modify_file: {
                file_path: "/workspace/src/app.ts",
                mutations: [{ type: "replace", start_line: 7 }],
              },
            },
          },
        })}
        isTaskDescription={false}
        worktreePath="/workspace"
        sessionId="requested-session"
        onOpenFile={onOpenFile}
      />,
    );

    expect(useOpenFileAtLine).toHaveBeenCalledWith(
      onOpenFile,
      7,
      "/workspace",
      "requested-session",
    );
  });

  it("opens agent plan file links with the renderer file opener", () => {
    const onOpenFile = vi.fn();

    render(
      <MessageRenderer
        comment={message({
          type: "agent_plan",
          content: "# Plan\n\nRead [AGENTS](/apps/web/AGENTS.md).",
        })}
        isTaskDescription={false}
        worktreePath="/workspace/kandev"
        onOpenFile={onOpenFile}
      />,
    );

    fireEvent.click(screen.getByRole("link", { name: "AGENTS" }));

    expect(onOpenFile).toHaveBeenCalledWith("apps/web/AGENTS.md");
    expect(fallbackOpenFile).not.toHaveBeenCalled();
  });
});
