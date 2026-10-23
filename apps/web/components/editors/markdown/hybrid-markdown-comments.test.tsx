import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { HybridMarkdownComments, type CommentSelectionLifecycle } from "./hybrid-markdown-comments";

function observable<T>(initial: T) {
  let value = initial;
  const observers = new Set<{
    handleChange: (observable: unknown) => void;
    endUpdate: () => void;
  }>();
  const result = {
    observers,
    get: () => value,
    set: (next: T) => {
      value = next;
      for (const observer of observers) {
        observer.handleChange(result);
        observer.endUpdate();
      }
    },
    addObserver: (observer: typeof observers extends Set<infer O> ? O : never) =>
      observers.add(observer),
    removeObserver: (observer: typeof observers extends Set<infer O> ? O : never) =>
      observers.delete(observer),
  };
  return result;
}

function createLifecycle() {
  const sourceText = observable({ value: "# Heading\r\n\r\nBody" });
  const caretRect = observable({ left: 100, top: 40, bottom: 60 });
  const readonlyMode = observable(false);
  const focus = vi.fn();
  const lifecycle = {
    active: true,
    model: {
      sourceText,
      readonlyMode,
      selection: observable({ isCollapsed: false, range: { start: 2, endExclusive: 9 } }),
      selectionSource: observable("user"),
      isSelecting: observable(false),
    },
    view: {
      caretRect,
      coordinateSpace: { capture: () => ({ toClientRect: (rect: unknown) => rect }) },
      focus,
    },
  } as unknown as CommentSelectionLifecycle;
  return { lifecycle, sourceText, caretRect, readonlyMode, focus };
}

const TRIGGER = "hybrid-markdown-comment-trigger";

function openComposer() {
  fireEvent.click(screen.getByTestId(TRIGGER));
  const textarea = screen.getByRole("textbox");
  fireEvent.change(textarea, { target: { value: "Review selection" } });
  return textarea;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("hybrid comment capture validity", () => {
  it("closes a capture after source replacement and rejects a submission before the observer flushes", async () => {
    const { lifecycle, sourceText, focus } = createLifecycle();
    const onComment = vi.fn();
    render(<HybridMarkdownComments lifecycle={lifecycle} enabled onComment={onComment} />);
    const textarea = openComposer();
    await act(async () => {
      sourceText.set({ value: "unrelated replacement bytes" });
      fireEvent.keyDown(textarea, { key: "Enter", ctrlKey: true });
    });
    expect(onComment).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(focus).not.toHaveBeenCalled();
  });

  it("invalidates a capture even when the source changes back to identical bytes", async () => {
    const { lifecycle, sourceText } = createLifecycle();
    render(<HybridMarkdownComments lifecycle={lifecycle} enabled onComment={vi.fn()} />);
    openComposer();
    const source = sourceText.get();
    await act(async () => {
      sourceText.set({ value: "temporary" });
      sourceText.set(source);
    });
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("clears hidden or read-only captures before the surface becomes editable again", () => {
    const { lifecycle } = createLifecycle();
    const { rerender } = render(
      <HybridMarkdownComments lifecycle={lifecycle} enabled onComment={vi.fn()} />,
    );
    openComposer();
    rerender(<HybridMarkdownComments lifecycle={lifecycle} enabled={false} onComment={vi.fn()} />);
    expect(screen.queryByRole("textbox")).toBeNull();
    rerender(<HybridMarkdownComments lifecycle={lifecycle} enabled onComment={vi.fn()} />);
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("closes a capture when the model becomes read-only and releases source subscriptions", async () => {
    const { lifecycle, readonlyMode, sourceText, focus } = createLifecycle();
    const onComment = vi.fn();
    const { unmount } = render(
      <HybridMarkdownComments lifecycle={lifecycle} enabled onComment={onComment} />,
    );
    const textarea = openComposer();
    expect(sourceText.observers.size).toBe(1);
    await act(async () => {
      readonlyMode.set(true);
      fireEvent.keyDown(textarea, { key: "Enter", ctrlKey: true });
    });
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(onComment).not.toHaveBeenCalled();
    expect(focus).not.toHaveBeenCalled();
    unmount();
    expect(sourceText.observers.size).toBe(0);
    expect(readonlyMode.observers.size).toBe(0);
  });

  it("clears captures owned by a replaced lifecycle", () => {
    const first = createLifecycle();
    const second = createLifecycle();
    const { rerender } = render(
      <HybridMarkdownComments lifecycle={first.lifecycle} enabled onComment={vi.fn()} />,
    );
    openComposer();
    rerender(<HybridMarkdownComments lifecycle={second.lifecycle} enabled onComment={vi.fn()} />);
    expect(screen.queryByRole("textbox")).toBeNull();
  });
});

describe("hybrid comment focus", () => {
  it("returns focus to the active editor only after an accepted comment", () => {
    const { lifecycle, focus } = createLifecycle();
    const onComment = vi.fn();
    render(<HybridMarkdownComments lifecycle={lifecycle} enabled onComment={onComment} />);
    fireEvent.keyDown(openComposer(), { key: "Enter", ctrlKey: true });
    expect(onComment).toHaveBeenCalledWith({ text: "Review selection", start: 2, endExclusive: 9 });
    expect(focus).toHaveBeenCalledOnce();
  });

  it("does not steal focus when the composer is dismissed", () => {
    const { lifecycle, focus } = createLifecycle();
    render(<HybridMarkdownComments lifecycle={lifecycle} enabled onComment={vi.fn()} />);
    openComposer();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(focus).not.toHaveBeenCalled();
  });
});

it("clamps and flips the measured trigger inside a scrolled visual viewport and follows keyboard resize", async () => {
  const viewport = Object.assign(new EventTarget(), {
    offsetLeft: 20,
    offsetTop: 40,
    width: 320,
    height: 240,
  });
  vi.stubGlobal("visualViewport", viewport);
  const removeViewportListener = vi.spyOn(viewport, "removeEventListener");
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    width: 80,
    height: 44,
  } as DOMRect);
  const { lifecycle, caretRect } = createLifecycle();
  caretRect.set({ left: 330, top: 255, bottom: 275 });
  const { unmount } = render(
    <HybridMarkdownComments lifecycle={lifecycle} enabled onComment={vi.fn()} />,
  );
  const trigger = screen.getByTestId(TRIGGER);
  await waitFor(() => expect(trigger.style.left).toBe("252px"));
  expect(trigger.style.top).toBe("203px");
  expect(trigger.className).toContain("max-md:h-11");
  await act(async () => {
    viewport.offsetTop = 80;
    viewport.height = 140;
    viewport.dispatchEvent(new Event("resize"));
  });
  expect(trigger.style.top).toBe("168px");
  await act(async () => {
    viewport.offsetLeft = 60;
    viewport.dispatchEvent(new Event("scroll"));
  });
  expect(trigger.style.left).toBe("292px");
  unmount();
  expect(removeViewportListener).toHaveBeenCalledWith("resize", expect.any(Function));
  expect(removeViewportListener).toHaveBeenCalledWith("scroll", expect.any(Function));
  viewport.dispatchEvent(new Event("resize"));
});
