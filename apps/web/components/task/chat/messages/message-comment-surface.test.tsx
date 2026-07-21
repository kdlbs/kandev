import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { createMessageTextAnchor } from "@/lib/chat/agent-message-comments";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageCommentSurface } from "./message-comment-surface";

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/components/task/plan-selection-popover", () => ({
  PlanSelectionPopover: ({
    selectedText,
    onAdd,
  }: {
    selectedText: string;
    onAdd: (feedback: string) => void;
  }) => (
    <div data-testid="comment-popover">
      <span>{selectedText}</span>
      <button type="button" onClick={() => onAdd("Make this concrete.")}>
        Save comment
      </button>
    </div>
  ),
}));

const SESSION_ID = "session-1";

function message(content: string): Message {
  return {
    id: "message-1",
    session_id: toSessionId(SESSION_ID),
    task_id: toTaskId("task-1"),
    author_type: "agent",
    type: "message",
    content,
    created_at: "2026-07-21T00:00:00Z",
  };
}

function resetComments() {
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
  sessionStorage.clear();
}

afterEach(() => {
  cleanup();
  resetComments();
});

describe("MessageCommentSurface", () => {
  it("anchors a pending selection to the same quote when message text shifts", () => {
    const original = "The settled answer contains detail.";
    const updated = "Intro. The settled answer contains detail.";
    const { rerender } = render(
      <MessageCommentSurface
        message={message(original)}
        sessionId={SESSION_ID}
        isTurnActive={false}
      >
        <span data-testid="message-text">{original}</span>
      </MessageCommentSurface>,
    );

    const text = screen.getByTestId("message-text").firstChild!;
    const range = document.createRange();
    const start = original.indexOf("settled answer");
    range.setStart(text, start);
    range.setEnd(text, start + "settled answer".length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseUp(screen.getByTestId("message-text").parentElement!);
    fireEvent.click(screen.getByTestId("agent-message-comment-trigger"));

    rerender(
      <MessageCommentSurface message={message(updated)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid="message-text">{updated}</span>
      </MessageCommentSurface>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Save comment" }));

    const saved = Object.values(useCommentsStore.getState().byId)[0];
    expect(saved?.source).toBe("agent-message");
    if (saved?.source !== "agent-message") throw new Error("Expected an agent message comment");
    expect(saved.selectedText).toBe("settled answer");
    expect(saved.anchor.start).toBe(updated.indexOf("settled answer"));
  });

  it("keeps React-owned text nodes under their rendered parent", () => {
    const content = "A settled answer.";
    const start = content.indexOf("settled");
    useCommentsStore.getState().addComment({
      id: "comment-1",
      sessionId: SESSION_ID,
      source: "agent-message",
      messageId: "message-1",
      selectedText: "settled",
      text: "Make this concrete.",
      createdAt: "2026-07-21T00:00:00Z",
      status: "pending",
      anchor: createMessageTextAnchor("message-1", content, start, start + "settled".length),
    });

    render(
      <MessageCommentSurface message={message(content)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid="react-owned-text">{content}</span>
      </MessageCommentSurface>,
    );

    const reactOwnedText = screen.getByTestId("react-owned-text");
    expect(
      Array.from(reactOwnedText.childNodes).every((node) => node.nodeType === Node.TEXT_NODE),
    ).toBe(true);
    expect(document.querySelector('.comment-badge[data-comment-id="comment-1"]')).not.toBeNull();
  });
});
