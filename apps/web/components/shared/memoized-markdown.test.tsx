import { useState } from "react";
import { fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const markdownSpy = vi.fn();

vi.mock("react-markdown", () => ({
  default: ({ children }: { children: string }) => {
    markdownSpy(children);
    return <div data-testid="md">{children}</div>;
  },
}));

// Stub the component overrides so the test stays light (no shiki / mermaid).
vi.mock("@/components/shared/markdown-components", () => ({
  markdownComponents: {},
  remarkPlugins: [],
}));

import { MemoizedMarkdown } from "./memoized-markdown";
import { __resetMarkdownCounters } from "@/lib/markdown/normalize-cache";

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
  afterEach(() => {
    markdownSpy.mockClear();
    __resetMarkdownCounters();
  });

  it("does not re-render markdown when the parent re-renders with same content", () => {
    const { getByText } = render(<Parent content="hello world" />);
    expect(markdownSpy).toHaveBeenCalledTimes(1);
    fireEvent.click(getByText("tick"));
    fireEvent.click(getByText("tick"));
    expect(markdownSpy).toHaveBeenCalledTimes(1);
  });

  it("re-renders markdown when content changes", () => {
    const { rerender } = render(<MemoizedMarkdown content="first" />);
    expect(markdownSpy).toHaveBeenCalledTimes(1);
    rerender(<MemoizedMarkdown content="second" />);
    expect(markdownSpy).toHaveBeenCalledTimes(2);
  });
});
