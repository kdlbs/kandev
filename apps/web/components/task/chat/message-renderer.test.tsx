import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MessageRenderer } from "./message-renderer";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

const fallbackOpenFile = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-panel-actions", () => ({
  usePanelActions: () => ({ openFile: fallbackOpenFile }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      turns: { bySession: {} },
      taskSessions: { items: {} },
      sessionModels: { bySessionId: {} },
    }),
  useAppStoreApi: () => ({ getState: () => ({}) }),
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

describe("MessageRenderer advisor feedback", () => {
  it("renders advisor feedback with a distinct label", () => {
    render(
      <MessageRenderer
        comment={message({
          type: "advisor_feedback",
          content: "Good point. Verify the ACP conversion path.",
          metadata: { source: "advisor", severity: "concern" },
        })}
        isTaskDescription={false}
      />,
    );

    expect(screen.getByText("OMP Advisor Feedback")).toBeTruthy();
    expect(screen.getByText("Good point. Verify the ACP conversion path.")).toBeTruthy();
  });
});
