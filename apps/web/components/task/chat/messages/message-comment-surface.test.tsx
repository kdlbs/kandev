import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef, type ReactNode } from "react";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { createMessageTextAnchor } from "@/lib/chat/agent-message-comments";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { SelectionCommentHostProvider } from "@/components/shared/selection-comment-dock";
import { MessageCommentSurface } from "./message-comment-surface";

const COMMENT_TEXT = vi.hoisted(() => "Make this concrete.");
const TOUCH_DRAWER = vi.hoisted(() => ({ enabled: false }));
const RESPONSIVE = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => TOUCH_DRAWER.enabled,
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ ...RESPONSIVE }),
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
    <div data-testid={COMMENT_POPOVER_TEST_ID}>
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

const COMMENT_TRIGGER_TEST_ID = "agent-message-comment-trigger";
const COMMENT_POPOVER_TEST_ID = "comment-popover";
const SESSION_ID = "session-1";
const MESSAGE_ID = "message-1";
const MESSAGE_TEXT_TEST_ID = "message-text";
const SELECTED_QUOTE = "settled answer";
const SETTLED_ANSWER = "A settled answer.";

function SelectionHost({ children }: { children: ReactNode }) {
  const ownerRef = useRef<HTMLDivElement>(null);
  const reserveRef = useRef<HTMLDivElement>(null);
  return (
    <SelectionCommentHostProvider ownerRef={ownerRef} reserveRef={reserveRef}>
      <div
        ref={(element) => {
          ownerRef.current = element;
          if (element) {
            vi.spyOn(element, "getBoundingClientRect").mockReturnValue({
              left: 20,
              top: 30,
              right: 340,
              bottom: 530,
              width: 320,
              height: 500,
              x: 20,
              y: 30,
              toJSON: () => ({}),
            });
          }
        }}
        data-testid="selection-owner"
      >
        <div ref={reserveRef}>{children}</div>
      </div>
    </SelectionCommentHostProvider>
  );
}

function message(content: string): Message {
  return {
    id: MESSAGE_ID,
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
    messageId: MESSAGE_ID,
    selectedText,
    text: COMMENT_TEXT,
    createdAt: "2026-07-21T00:00:00Z",
    status: "pending",
    anchor: createMessageTextAnchor(MESSAGE_ID, content, start, start + selectedText.length),
  });
}

afterEach(() => {
  cleanup();
  resetComments();
  TOUCH_DRAWER.enabled = false;
  RESPONSIVE.isMobile = false;
  RESPONSIVE.isFinePointer = true;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("MessageCommentSurface selection capture", () => {
  it("keeps the touch Comment action independent of the selection rectangle", () => {
    TOUCH_DRAWER.enabled = true;
    RESPONSIVE.isMobile = true;
    const content = SETTLED_ANSWER;
    let selectionTop = 180;
    vi.spyOn(Range.prototype, "getBoundingClientRect").mockImplementation(
      () => new DOMRect(100, selectionTop, 12, 18),
    );
    render(
      <SelectionHost>
        <MessageCommentSurface
          message={message(content)}
          sessionId={SESSION_ID}
          isTurnActive={false}
        >
          <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
        </MessageCommentSurface>
      </SelectionHost>,
    );
    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
    const range = document.createRange();
    const start = content.indexOf(SELECTED_QUOTE);
    range.setStart(text, start);
    range.setEnd(text, start + SELECTED_QUOTE.length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.touchEnd(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    const dockOrTrigger = () =>
      screen.queryByTestId("selection-comment-dock") ??
      screen.getByTestId(COMMENT_TRIGGER_TEST_ID).parentElement;
    const firstPosition = {
      left: dockOrTrigger()?.getAttribute("style"),
      top: dockOrTrigger()?.getAttribute("style"),
    };

    selectionTop = 400;
    fireEvent.touchEnd(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);

    expect(dockOrTrigger()?.getAttribute("style")).toBe(firstPosition.top);
  });
});

describe("MessageCommentSurface delayed touch selection", () => {
  it("updates the touch quote after a delayed native selection change", () => {
    TOUCH_DRAWER.enabled = true;
    RESPONSIVE.isMobile = true;
    const content = "A settled answer and final result.";
    render(
      <SelectionHost>
        <MessageCommentSurface
          message={message(content)}
          sessionId={SESSION_ID}
          isTurnActive={false}
        >
          <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
        </MessageCommentSurface>
      </SelectionHost>,
    );
    const messageText = screen.getByTestId(MESSAGE_TEXT_TEST_ID);
    const text = messageText.firstChild!;
    const selection = window.getSelection()!;
    const select = (quote: string) => {
      const start = content.indexOf(quote);
      const range = document.createRange();
      range.setStart(text, start);
      range.setEnd(text, start + quote.length);
      selection.removeAllRanges();
      selection.addRange(range);
    };

    fireEvent.touchStart(messageText);
    select("settled answer");
    fireEvent.touchEnd(messageText.parentElement!);
    expect(screen.getByTestId(COMMENT_TRIGGER_TEST_ID)).not.toBeNull();

    select("final result");
    fireEvent(document, new Event("selectionchange"));
    fireEvent.click(screen.getByTestId(COMMENT_TRIGGER_TEST_ID));

    expect(screen.getByTestId("agent-message-comment-drawer").textContent).toContain(
      "final result",
    );
  });
});

describe("MessageCommentSurface collapsed touch selection", () => {
  it("clears the touch action when the active selection collapses", () => {
    TOUCH_DRAWER.enabled = true;
    RESPONSIVE.isMobile = true;
    const content = SETTLED_ANSWER;
    render(
      <SelectionHost>
        <MessageCommentSurface
          message={message(content)}
          sessionId={SESSION_ID}
          isTurnActive={false}
        >
          <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
        </MessageCommentSurface>
      </SelectionHost>,
    );
    const messageText = screen.getByTestId(MESSAGE_TEXT_TEST_ID);
    const text = messageText.firstChild!;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, SELECTED_QUOTE.length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.touchStart(messageText);
    fireEvent.touchEnd(messageText.parentElement!);
    expect(screen.getByTestId(COMMENT_TRIGGER_TEST_ID)).not.toBeNull();

    selection.removeAllRanges();
    fireEvent(document, new Event("selectionchange"));

    expect(screen.queryByTestId(COMMENT_TRIGGER_TEST_ID)).toBeNull();
  });
});

describe("MessageCommentSurface eligibility", () => {
  it.each(["outside", "cross-message", "active turn", "missing session"])(
    "does not offer a comment for an invalid selection: %s",
    (scenario) => {
      const content = SETTLED_ANSWER;
      render(
        <>
          <MessageCommentSurface
            message={message(content)}
            sessionId={scenario === "missing session" ? null : SESSION_ID}
            isTurnActive={scenario === "active turn"}
          >
            <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
          </MessageCommentSurface>
          <MessageCommentSurface
            message={{ ...message(content), id: "message-2" }}
            sessionId={SESSION_ID}
            isTurnActive={false}
          >
            <span data-testid="second-message">{content}</span>
          </MessageCommentSurface>
          <span data-testid="outside-text">Outside the messages.</span>
        </>,
      );
      const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
      const range = document.createRange();
      range.setStart(text, 0);
      range.setEnd(text, content.length);
      if (scenario === "outside") range.selectNodeContents(screen.getByTestId("outside-text"));
      if (scenario === "cross-message")
        range.setEnd(screen.getByTestId("second-message").firstChild!, content.length);
      const selection = window.getSelection()!;
      selection.removeAllRanges();
      selection.addRange(range);
      fireEvent.mouseDown(screen.getByTestId(MESSAGE_TEXT_TEST_ID));
      fireEvent.mouseUp(document.body);

      expect(screen.queryByTestId(COMMENT_TRIGGER_TEST_ID)).toBeNull();
    },
  );

  it("captures a message selection when the mouse is released in surrounding padding", () => {
    const content = "The settled answer includes context.";
    render(
      <div data-testid="message-padding">
        <MessageCommentSurface
          message={message(content)}
          sessionId={SESSION_ID}
          isTurnActive={false}
        >
          <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
        </MessageCommentSurface>
      </div>,
    );

    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, 18);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseDown(screen.getByTestId(MESSAGE_TEXT_TEST_ID));
    fireEvent.mouseUp(screen.getByTestId("message-padding"));

    fireEvent.click(screen.getByTestId(COMMENT_TRIGGER_TEST_ID));
    // Releasing the mouse in the editor must preserve its open state and quote.
    fireEvent.mouseUp(screen.getByTestId(COMMENT_POPOVER_TEST_ID));
    fireEvent.click(screen.getByRole("button", { name: "Save comment" }));
    const saved = Object.values(useCommentsStore.getState().byId)[0];
    expect(saved).toMatchObject({
      messageId: MESSAGE_ID,
      selectedText: "The settled answer",
      text: COMMENT_TEXT,
    });
  });
});

describe("MessageCommentSurface selection dismissal", () => {
  it("does not restore a dismissed trigger after an unrelated mouse click", () => {
    const content = "The settled answer includes context.";
    render(
      <>
        <MessageCommentSurface
          message={message(content)}
          sessionId={SESSION_ID}
          isTurnActive={false}
        >
          <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
        </MessageCommentSurface>
        <button onMouseDown={(event) => event.preventDefault()}>Unrelated control</button>
      </>,
    );
    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID);
    const range = document.createRange();
    range.selectNodeContents(text);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseDown(text);
    fireEvent.mouseUp(text);
    expect(screen.getByTestId(COMMENT_TRIGGER_TEST_ID)).not.toBeNull();

    const control = screen.getByRole("button", { name: "Unrelated control" });
    fireEvent.pointerDown(control);
    expect(screen.queryByTestId(COMMENT_TRIGGER_TEST_ID)).toBeNull();
    fireEvent.mouseDown(control);
    fireEvent.mouseUp(control);
    fireEvent.click(control);

    expect(selection.toString()).toBe(content);
    expect(screen.queryByTestId(COMMENT_TRIGGER_TEST_ID)).toBeNull();
  });
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
    fireEvent.mouseDown(screen.getByTestId(MESSAGE_TEXT_TEST_ID));
    fireEvent.mouseUp(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    fireEvent.click(screen.getByTestId(COMMENT_TRIGGER_TEST_ID));

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
    const content = SETTLED_ANSWER;
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
    fireEvent.mouseDown(screen.getByTestId(MESSAGE_TEXT_TEST_ID));
    fireEvent.mouseUp(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    fireEvent.click(screen.getByTestId(COMMENT_TRIGGER_TEST_ID));

    rerender(
      <MessageCommentSurface message={message(updated)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{updated}</span>
      </MessageCommentSurface>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Save comment" }));

    expect(screen.getByTestId(COMMENT_POPOVER_TEST_ID)).not.toBeNull();
    expect(Object.values(useCommentsStore.getState().byId)).toHaveLength(0);
    expect(screen.getByRole("alert").textContent).toBe(
      "The agent response changed. Select the text again.",
    );
  });
});

describe("MessageCommentSurface layering", () => {
  it("keeps the selection trigger below overlay popovers", () => {
    const content = "A selection sits above the chat input.";
    const selectedText = "selection";
    render(
      <MessageCommentSurface message={message(content)} sessionId={SESSION_ID} isTurnActive={false}>
        <span data-testid={MESSAGE_TEXT_TEST_ID}>{content}</span>
      </MessageCommentSurface>,
    );

    const text = screen.getByTestId(MESSAGE_TEXT_TEST_ID).firstChild!;
    const range = document.createRange();
    const start = content.indexOf(selectedText);
    range.setStart(text, start);
    range.setEnd(text, start + selectedText.length);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);
    fireEvent.mouseDown(screen.getByTestId(MESSAGE_TEXT_TEST_ID));
    fireEvent.mouseUp(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);

    expect(
      screen
        .getByRole("button", { name: "Comment on selection" })
        .parentElement?.classList.contains("z-40"),
    ).toBe(true);
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
    fireEvent.touchEnd(screen.getByTestId(MESSAGE_TEXT_TEST_ID).parentElement!);
    fireEvent.click(screen.getByTestId(COMMENT_TRIGGER_TEST_ID));
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
    const content = SETTLED_ANSWER;
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

    expect(screen.queryByTestId(COMMENT_POPOVER_TEST_ID)).toBeNull();
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
    expect(screen.queryByTestId(COMMENT_POPOVER_TEST_ID)).toBeNull();
  });
});
