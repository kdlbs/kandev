import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MessageRenderer } from "./message-renderer";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

const fallbackOpenFile = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({ openFile: fallbackOpenFile }),
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
