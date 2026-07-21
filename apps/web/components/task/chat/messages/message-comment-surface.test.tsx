import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { createMessageTextAnchor } from "@/lib/chat/agent-message-comments";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageCommentSurface } from "./message-comment-surface";

const COMMENT_TEXT = vi.hoisted(() => "Make this concrete.");
const TOUCH_DRAWER = vi.hoisted(() => ({ enabled: false }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => TOUCH_DRAWER.enabled,
}));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/components/task/plan-selection-popover", () => ({
  PlanSelectionPopover: ({
    selectedText,
    onAdd,
    onClose,
    errorMessage,
  }: {
    selectedText: string;
    onAdd: (feedback: string) => boolean | void;
    onClose: () => void;
    errorMessage?: string | null;
  }) => (
    <div data-testid="comment-popover">
      <span>{selectedText}</span>
      {errorMessage ? <p role="alert">{errorMessage}</p> : null}
      <button
        type="button"
        onClick={() => {
          if (onAdd(COMMENT_TEXT) !== false) onClose();
        }}
      >
        Save comment
      </button>
    </div>
  ),
}));

const SESSION_ID = "session-1";
const MESSAGE_TEXT_TEST_ID = "message-text";
const SELECTED_QUOTE = "settled answer";

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

function addPendingComment(content: string, selectedText: string) {
  const start = content.indexOf(selectedText);
  useCommentsStore.getState().addComment({
    id: "comment-1",
    sessionId: SESSION_ID,
    source: "agent-message",
    messageId: "message-1",
    selectedText,
    text: COMMENT_TEXT,
    createdAt: "2026-07-21T00:00:00Z",
    status: "pending",
    anchor: createMessageTextAnchor("message-1", content, start, start + selectedText.length),
  });
}

afterEach(() => {
  cleanup();
  resetComments();
  TOUCH_DRAWER.enabled = false;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
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
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{original}</span>
      </MessageCommentSurface>,
    );

    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
    const range = document.createRange();
    const start = original.indexOf(SELECTED_QUOTE);
    range.setStart(text, start);
    range.setEnd(text, start + SELECTED_QUOTE.length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseUp(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    fireEvent.click(screen.getByTestId("agent-message-comment-trigger"));

    rerender(
      <MessageCommentSurface message={message(updated)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{updated}</span>
      </MessageCommentSurface>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Save comment" }));

    const saved = Object.values(useCommentsStore.getState().byId)[0];
    expect(saved?.source).toBe("agent-message");
    if (saved?.source !== "agent-message") throw new Error("Expected an agent message comment");
    expect(saved.selectedText).toBe(SELECTED_QUOTE);
    expect(saved.anchor.start).toBe(updated.indexOf(SELECTED_QUOTE));
  });

  it("keeps React-owned text nodes under their rendered parent", () => {
    const content = "A settled answer.";
    addPendingComment(content, "settled");

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

  it("keeps feedback open and reports when the selected quote was rewritten", () => {
    const original = "The settled answer contains detail.";
    const updated = "The response was completely rewritten.";
    const { rerender } = render(
      <MessageCommentSurface
        message={message(original)}
        sessionId={SESSION_ID}
        isTurnActive={false}
      >
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{original}</span>
      </MessageCommentSurface>,
    );

    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
    const range = document.createRange();
    const start = original.indexOf(SELECTED_QUOTE);
    range.setStart(text, start);
    range.setEnd(text, start + SELECTED_QUOTE.length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseUp(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    fireEvent.click(screen.getByTestId("agent-message-comment-trigger"));

    rerender(
      <MessageCommentSurface message={message(updated)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{updated}</span>
      </MessageCommentSurface>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Save comment" }));

    expect(screen.getByTestId("comment-popover")).not.toBeNull();
    expect(Object.values(useCommentsStore.getState().byId)).toHaveLength(0);
    expect(screen.getByRole("alert").textContent).toBe(
      "The agent response changed. Select the text again.",
    );
  });
});

describe("MessageCommentSurface mobile drawer", () => {
  it("keeps mobile feedback open when the selected quote was rewritten", () => {
    TOUCH_DRAWER.enabled = true;
    const original = "The settled answer contains detail.";
    const updated = "The response was completely rewritten.";
    const { rerender } = render(
      <MessageCommentSurface
        message={message(original)}
        sessionId={SESSION_ID}
        isTurnActive={false}
      >
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{original}</span>
      </MessageCommentSurface>,
    );

    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
    const range = document.createRange();
    const start = original.indexOf(SELECTED_QUOTE);
    range.setStart(text, start);
    range.setEnd(text, start + SELECTED_QUOTE.length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseUp(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    fireEvent.click(screen.getByTestId("agent-message-comment-trigger"));
    const input = screen.getByTestId("agent-message-comment-input");
    fireEvent.change(input, { target: { value: COMMENT_TEXT } });

    rerender(
      <MessageCommentSurface message={message(updated)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{updated}</span>
      </MessageCommentSurface>,
    );
    fireEvent.click(screen.getByTestId("agent-message-comment-add"));

    expect(screen.getByTestId("agent-message-comment-drawer")).not.toBeNull();
    expect((input as HTMLTextAreaElement).value).toBe(COMMENT_TEXT);
    expect(screen.getByRole("alert").textContent).toBe(
      "The agent response changed. Select the text again.",
    );
  });
});

describe("MessageCommentSurface fallback highlights", () => {
  it("renders visible ranges when the CSS Highlight API is unavailable", () => {
    vi.stubGlobal("CSS", {});
    vi.stubGlobal("Highlight", undefined);
    const content = "A settled answer.";
    addPendingComment(content, "settled");

    render(
      <MessageCommentSurface message={message(content)} sessionId={SESSION_ID} isTurnActive={false}>
        <span>{content}</span>
      </MessageCommentSurface>,
    );

    expect(
      document.querySelector('[data-agent-message-comment-fallback][data-comment-id="comment-1"]'),
    ).not.toBeNull();
  });
});

describe("MessageCommentSurface highlighted interactions", () => {
  it("does not open a highlighted comment while text is selected", () => {
    vi.spyOn(Range.prototype, "getClientRects").mockReturnValue([
      new DOMRect(0, 0, 100, 20),
    ] as unknown as DOMRectList);
    const content = "Read the settled answer.";
    addPendingComment(content, "settled answer");
    const { container } = render(
      <MessageCommentSurface message={message(content)} sessionId={SESSION_ID} isTurnActive={false}>
        <span>{content}</span>
      </MessageCommentSurface>,
    );
    vi.spyOn(window, "getSelection").mockReturnValue({ isCollapsed: false } as Selection);

    fireEvent.click(container.querySelector("[data-agent-message-body]")!, {
      clientX: 10,
      clientY: 10,
    });

    expect(screen.queryByTestId("comment-popover")).toBeNull();
  });

  it("preserves native link clicks inside a highlighted range", () => {
    vi.spyOn(Range.prototype, "getClientRects").mockReturnValue([
      new DOMRect(0, 0, 100, 20),
    ] as unknown as DOMRectList);
    const content = "Read the docs.";
    addPendingComment(content, "docs");

    render(
      <MessageCommentSurface message={message(content)} sessionId={SESSION_ID} isTurnActive={false}>
        <a href="#docs">{content}</a>
      </MessageCommentSurface>,
    );

    expect(fireEvent.click(screen.getByRole("link"), { clientX: 10, clientY: 10 })).toBe(true);
    expect(screen.queryByTestId("comment-popover")).toBeNull();
  });
});
