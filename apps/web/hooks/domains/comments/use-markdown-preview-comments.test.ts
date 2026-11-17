import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { SOURCE_END_ATTR, SOURCE_START_ATTR } from "@/lib/markdown/source-line-ranges";
import { isIgnoredTarget, useMarkdownPreviewComments } from "./use-markdown-preview-comments";

describe("isIgnoredTarget", () => {
  it("treats SVG descendants inside interactive elements as ignored", () => {
    const button = document.createElement("button");
    button.innerHTML = "<svg><path /></svg>";

    expect(isIgnoredTarget(button.querySelector("path"))).toBe(true);
  });

  it("does not ignore plain non-interactive elements", () => {
    expect(isIgnoredTarget(document.createElement("span"))).toBe(false);
  });
});

const TOUCH = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => TOUCH.enabled }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("./use-run-comment", () => ({ useRunComment: () => ({ runComment: vi.fn() }) }));

const content = "# Install\n\nRun pnpm install from apps.";
let frames: FrameRequestCallback[] = [];
let root: HTMLDivElement;
let outside: HTMLButtonElement | null = null;

beforeEach(() => {
  frames = [];
  outside = null;
  vi.stubGlobal("requestAnimationFrame", (frame: FrameRequestCallback) => frames.push(frame));
  vi.stubGlobal("cancelAnimationFrame", () => {});
  root = document.createElement("div");
  root.innerHTML = `<p ${SOURCE_START_ATTR}="3" ${SOURCE_END_ATTR}="3">Run pnpm install from apps.</p>`;
  document.body.appendChild(root);
});

afterEach(() => {
  cleanup();
  TOUCH.enabled = false;
  window.getSelection()?.removeAllRanges();
  outside?.remove();
  outside = null;
  root.remove();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function selectAndFlush(start: number, end: number) {
  const text = root.querySelector("p")!.firstChild!;
  const range = document.createRange();
  range.setStart(text, start);
  range.setEnd(text, end);
  const selection = window.getSelection()!;
  selection.removeAllRanges();
  selection.addRange(range);
  act(() => {
    document.dispatchEvent(new Event("selectionchange"));
    const pending = frames;
    frames = [];
    pending.forEach((frame) => frame(0));
  });
}

function renderPreviewHook() {
  const rootRef = { current: root };
  return renderHook(() =>
    useMarkdownPreviewComments({
      path: "README.md",
      repositoryName: "repo",
      content,
      sessionId: "session-1",
      taskId: "task-1",
      enabled: true,
      rootRef,
    }),
  );
}

describe("useMarkdownPreviewComments selection tracking", () => {
  it("follows native selection changes and keeps the selection on scroll", () => {
    TOUCH.enabled = true;
    const { result } = renderPreviewHook();

    selectAndFlush(4, 8);
    expect(result.current.touch).toBe(true);
    expect(result.current.currentSelection?.selectedText).toBe("pnpm");
    selectAndFlush(4, 16);
    expect(result.current.currentSelection?.selectedText).toBe("pnpm install");

    act(() => result.current.dismissOverlays());
    expect(result.current.currentSelection?.selectedText).toBe("pnpm install");

    selectAndFlush(4, 4);
    expect(result.current.currentSelection).toBeNull();
  });

  it("keeps the touch selection when a swipe starts on selected text", () => {
    TOUCH.enabled = true;
    const { result } = renderPreviewHook();
    selectAndFlush(4, 8);
    expect(result.current.currentSelection?.selectedText).toBe("pnpm");

    act(() => {
      root.querySelector("p")!.dispatchEvent(new Event("touchstart", { bubbles: true }));
    });

    expect(result.current.currentSelection?.selectedText).toBe("pnpm");
  });

  it("ignores selectionchange with a fine pointer", () => {
    const { result } = renderPreviewHook();

    selectAndFlush(4, 8);

    expect(result.current.touch).toBe(false);
    expect(result.current.currentSelection).toBeNull();
  });
});

describe("useMarkdownPreviewComments pending selection ownership", () => {
  it("does not restore a dismissed selection from a pending touch capture", () => {
    TOUCH.enabled = true;
    const outsideButton = document.createElement("button");
    outside = outsideButton;
    document.body.appendChild(outsideButton);
    const { result } = renderPreviewHook();

    selectAndFlush(4, 8);
    vi.useFakeTimers();
    act(() => {
      root.dispatchEvent(new Event("touchstart", { bubbles: true }));
      root.dispatchEvent(new Event("touchend", { bubbles: true }));
    });
    expect(result.current.currentSelection?.selectedText).toBe("pnpm");

    act(() => outsideButton.dispatchEvent(new Event("pointerdown", { bubbles: true })));
    expect(result.current.currentSelection).toBeNull();
    act(() => vi.advanceTimersByTime(20));

    expect(result.current.currentSelection).toBeNull();
    expect(window.getSelection()?.toString()).toBe("pnpm");
  });

  it("cancels pending selection capture while the composer owns the quote", () => {
    TOUCH.enabled = true;
    const { result } = renderPreviewHook();

    selectAndFlush(4, 8);
    vi.useFakeTimers();
    act(() => root.dispatchEvent(new Event("touchend", { bubbles: true })));
    act(() => result.current.openComposer());
    expect(result.current.textSelection?.selectedText).toBe("pnpm");

    act(() => vi.advanceTimersByTime(20));
    expect(result.current.currentSelection).toBeNull();
    expect(result.current.textSelection?.selectedText).toBe("pnpm");

    act(() => result.current.closeComposer());
    act(() => vi.advanceTimersByTime(20));
    expect(result.current.currentSelection).toBeNull();
    expect(result.current.textSelection).toBeNull();
  });

  it("cancels pending selection capture when the preview identity changes", () => {
    TOUCH.enabled = true;
    const rootRef = { current: root };
    const { result, rerender } = renderHook(
      ({ path }: { path: string }) =>
        useMarkdownPreviewComments({
          path,
          repositoryName: "repo",
          content,
          sessionId: "session-1",
          taskId: "task-1",
          enabled: true,
          rootRef,
        }),
      { initialProps: { path: "README.md" } },
    );

    selectAndFlush(4, 8);
    vi.useFakeTimers();
    act(() => {
      root.dispatchEvent(new Event("touchstart", { bubbles: true }));
      root.dispatchEvent(new Event("touchend", { bubbles: true }));
    });
    rerender({ path: "CONTRIBUTING.md" });
    act(() => vi.advanceTimersByTime(20));

    expect(result.current.currentSelection).toBeNull();
    expect(result.current.textSelection).toBeNull();
  });
});

describe("useMarkdownPreviewComments composer lifecycle", () => {
  it("keeps an open comment draft when the viewport resizes", () => {
    TOUCH.enabled = true;
    const { result } = renderPreviewHook();
    selectAndFlush(4, 8);
    act(() => result.current.openComposer());
    expect(result.current.textSelection?.selectedText).toBe("pnpm");

    act(() => result.current.dismissForResize());

    expect(result.current.textSelection?.selectedText).toBe("pnpm");
  });

  it("keeps the touch composer open when dock removal scrolls the preview", () => {
    TOUCH.enabled = true;
    const { result } = renderPreviewHook();
    selectAndFlush(4, 8);
    act(() => result.current.openComposer());

    act(() => result.current.dismissOverlays());

    expect(result.current.currentSelection).toBeNull();
    expect(result.current.textSelection?.selectedText).toBe("pnpm");
  });
});
