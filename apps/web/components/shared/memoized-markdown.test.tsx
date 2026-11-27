import { createContext, useState } from "react";
import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const markdownSpy = vi.fn();

vi.mock("react-markdown", () => ({
  default: ({ children }: { children: string }) => {
    markdownSpy(children);
    return <div data-testid="md">{children}</div>;
  },
}));

// Stub the component overrides so the test stays light (no shiki / mermaid).
vi.mock("@/components/shared/markdown-components", () => ({
  MarkdownFileLinkContext: createContext({}),
  MarkdownTaskContext: createContext(null),
  markdownComponents: {},
  rehypePlugins: [],
  remarkPlugins: [],
}));

let MemoizedMarkdown: typeof import("./memoized-markdown").MemoizedMarkdown;
import { markdownComponents } from "./markdown-components";

function Parent({ content }: { content: string }) {
  const [tick, setTick] = useState(0);
  return (
    <div data-tick={tick}>
      <button onClick={() => setTick((t) => t + 1)}>tick</button>
      <MemoizedMarkdown content={content} />
    </div>
  );
}

describe("MemoizedMarkdown", () => {
  beforeEach(async () => {
    vi.resetModules();
    ({ MemoizedMarkdown } = await import("./memoized-markdown"));
  });

  afterEach(() => {
    cleanup();
    markdownSpy.mockClear();
  });

  it("does not re-render markdown when the parent re-renders with same content", () => {
    const { getByText } = render(<Parent content="hello world" />);
    expect(markdownSpy).toHaveBeenCalledTimes(1);
    fireEvent.click(getByText("tick"));
    fireEvent.click(getByText("tick"));
    expect(markdownSpy).toHaveBeenCalledTimes(1);
  });

  it("re-renders markdown when content changes", () => {
    const { rerender } = render(<MemoizedMarkdown content="hello world" />);
    expect(markdownSpy).toHaveBeenCalledTimes(1);
    rerender(<MemoizedMarkdown content="second" />);
    expect(markdownSpy).toHaveBeenCalledTimes(2);
  });

  it("reuses unchanged Markdown after a task transcript unmounts and returns", () => {
    const first = render(<MemoizedMarkdown content="A completed delivery review" />);
    first.unmount();
    const second = render(<MemoizedMarkdown content="A completed delivery review" />);

    expect(second.getByText("A completed delivery review")).toBeTruthy();
    expect(markdownSpy).toHaveBeenCalledTimes(1);
  });

  it("reuses user prompts that explicitly pass the standard renderers", () => {
    const props = { content: "A prompt without entity mentions", components: markdownComponents };
    const first = render(<MemoizedMarkdown {...props} />);
    first.unmount();
    const second = render(<MemoizedMarkdown {...props} />);
    expect(second.getByText(props.content)).toBeTruthy();
    expect(markdownSpy).toHaveBeenCalledOnce();
  });

  it("evicts old renderings when many distinct messages have been visited", () => {
    const content = "Old delivery rendering with bounded retention";
    const view = render(<MemoizedMarkdown content={content} />);
    for (let index = 0; index < 256; index++) {
      view.rerender(<MemoizedMarkdown content={`Other delivery rendering ${index}`} />);
    }
    markdownSpy.mockClear();
    view.rerender(<MemoizedMarkdown content={content} />);
    expect(markdownSpy).toHaveBeenCalledOnce();
  });

  it("bounds retained content size as well as entry count", () => {
    const content = `First large message ${"a".repeat(200_000)}`;
    const view = render(<MemoizedMarkdown content={content} />);
    view.rerender(<MemoizedMarkdown content={"b".repeat(200_000)} />);
    view.rerender(<MemoizedMarkdown content={"c".repeat(200_000)} />);
    markdownSpy.mockClear();
    view.rerender(<MemoizedMarkdown content={content} />);
    expect(markdownSpy).toHaveBeenCalledOnce();
  });

  it("does not retain an individual oversized message", () => {
    const content = "Oversized delivery message ".repeat(25_000);
    const first = render(<MemoizedMarkdown content={content} />);
    first.unmount();
    render(<MemoizedMarkdown content={content} />);
    expect(markdownSpy).toHaveBeenCalledTimes(2);
  });
});
