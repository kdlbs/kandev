import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createContext } from "react";
const customSpanFixture = vi.hoisted(() => ({ enabled: false }));

import { MemoizedMarkdown } from "./memoized-markdown";
vi.mock("@/components/shared/markdown-components", () => ({
  MarkdownFileLinkContext: createContext({}),
  MarkdownTaskContext: createContext(null),
  markdownComponents: {},
  rehypePlugins: [],
  remarkPlugins: [
    () => (tree: { children: { data?: object }[] }) => {
      if (customSpanFixture.enabled) tree.children[0].data = { hName: "span" };
    },
  ],
}));
const originalAnimate = Object.getOwnPropertyDescriptor(Element.prototype, "animate");
const motionSelector = "[data-chat-text-motion]";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  customSpanFixture.enabled = false;
  if (originalAnimate) Object.defineProperty(Element.prototype, "animate", originalAnimate);
  else Reflect.deleteProperty(Element.prototype, "animate");
});

// @covers AC-UI-CHAT-MOTION-001.1, AC-UI-CHAT-MOTION-001.4
describe("chat Markdown reveals", () => {
  it("reveals only new prose while preserving formatting, selection text and code", () => {
    const animate = vi.fn(() => ({ cancel: vi.fn() }));
    Object.defineProperty(Element.prototype, "animate", { value: animate, configurable: true });
    const view = render(<MemoizedMarkdown content="Hello" animateText />);
    expect(animate).not.toHaveBeenCalled();
    view.rerender(<MemoizedMarkdown content="Hello **world** `code`" animateText />);
    expect(view.container.textContent).toBe("Hello world code");
    expect(view.container.querySelector("strong")?.textContent).toBe("world");
    expect(view.container.querySelector("code [data-chat-text-motion]")).toBeNull();
    expect(view.container.querySelectorAll(motionSelector).length).toBeGreaterThan(0);
    expect(animate).toHaveBeenCalled();
    const selection = window.getSelection()!;
    const range = document.createRange();
    range.selectNodeContents(view.container);
    selection.addRange(range);
    expect(selection.toString()).toBe("Hello world code");
    selection.removeAllRanges();
  });
  it("cancels active effects on disable and does not replay on enable or rewrite", () => {
    const cancel = vi.fn();
    const animate = vi.fn(() => ({ cancel }));
    Object.defineProperty(Element.prototype, "animate", { value: animate, configurable: true });
    const view = render(<MemoizedMarkdown content="old" animateText />);
    view.rerender(<MemoizedMarkdown content="old new" animateText />);
    expect(animate).toHaveBeenCalled();
    view.rerender(<MemoizedMarkdown content="old new" animateText={false} />);
    expect(cancel).toHaveBeenCalled();
    animate.mockClear();
    view.rerender(<MemoizedMarkdown content="old new" animateText />);
    view.rerender(<MemoizedMarkdown content="replacement" animateText />);
    expect(animate).not.toHaveBeenCalled();
    expect(view.container.textContent).toBe("replacement");
  });
  it("compacts finished ranges after a burst and leaves no timers on unmount", () => {
    vi.useFakeTimers();
    const view = render(<MemoizedMarkdown content="a" animateText />);
    for (let i = 1; i < 40; i++)
      view.rerender(<MemoizedMarkdown content={"a".repeat(i + 1)} animateText />);
    expect(view.container.querySelectorAll(motionSelector).length).toBeLessThanOrEqual(1);
    act(() => vi.advanceTimersByTime(160));
    expect(view.container.querySelectorAll(motionSelector)).toHaveLength(0);
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
    vi.useRealTimers();
  });
});

it("retains the selected text while a reveal finishes and more text arrives", () => {
  vi.useFakeTimers();
  const view = render(<MemoizedMarkdown content="old" animateText />);
  view.rerender(<MemoizedMarkdown content="old selected" animateText />);
  const span = view.container.querySelector(motionSelector)!;
  const selection = window.getSelection()!;
  const range = document.createRange();
  range.selectNodeContents(span);
  selection.addRange(range);
  act(() => vi.advanceTimersByTime(160));
  expect(view.container.contains(span)).toBe(true);
  view.rerender(<MemoizedMarkdown content="old selected tail" animateText />);
  expect(view.container.contains(span)).toBe(true);
  expect(selection.toString()).toBe(" selected");
  act(() => {
    selection.removeAllRanges();
    document.dispatchEvent(new Event("selectionchange"));
  });
  expect(view.container.querySelector(motionSelector)).toBeNull();
  view.unmount();
  vi.useRealTimers();
});

it("preserves the caller's renderer for unmarked spans during text motion", () => {
  customSpanFixture.enabled = true;
  const components = {
    span: ({ children }: { children?: import("react").ReactNode }) => (
      <span data-custom-span>{children}</span>
    ),
  };
  const view = render(<MemoizedMarkdown content="old" components={components} animateText />);
  expect(view.container.querySelector("[data-custom-span]")?.textContent).toBe("old");
  view.rerender(<MemoizedMarkdown content="old new" components={components} animateText />);
  expect(view.container.querySelector("[data-custom-span]")?.textContent).toBe("old new");
  expect(view.container.querySelector("[data-custom-span] [data-chat-text-motion]")).not.toBeNull();
});
