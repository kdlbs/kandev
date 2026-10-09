import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MarkdownFileLinkContext } from "./markdown-components";
import { MarkdownPreviewRenderer } from "@/components/task/markdown-preview-content";
import { MarkdownComment } from "@/components/task/simple/markdown-comment";
import { MemoizedMarkdown } from "./memoized-markdown";

afterEach(cleanup);

// @covers AC-UI-MARKDOWN-MATH-001.3, AC-UI-MARKDOWN-MATH-002.4
describe("sanitized Markdown math renderers", () => {
  it("renders formulas in file previews while stripping executable HTML", () => {
    render(
      <MarkdownFileLinkContext.Provider value={{ onOpenFile: vi.fn() }}>
        <MarkdownPreviewRenderer
          content={[
            "Energy: $E = mc^2$",
            "",
            "$$",
            "\\frac{a}{b}",
            "$$",
            "",
            "$\\href{javascript:alert(1)}{click}$",
            "",
            '[unsafe](javascript:alert("xss"))',
            "",
            '<script>alert("xss")</script>',
          ].join("\n")}
        />
      </MarkdownFileLinkContext.Provider>,
    );

    expect(document.querySelector(".katex")).not.toBeNull();
    expect(document.querySelector(".katex-display")).not.toBeNull();
    expect(document.querySelector('a[href^="javascript:"]')).toBeNull();
    expect(document.querySelector("script")).toBeNull();
  });

  it("renders formulas in sanitized comments", () => {
    render(<MarkdownComment content="Energy: $E = mc^2$" />);

    expect(document.querySelector(".katex")).not.toBeNull();
  });

  // @covers AC-UI-MARKDOWN-MATH-001.5, AC-UI-MARKDOWN-MATH-001.6
  it("does not add chat motion spans inside generated KaTeX markup", () => {
    const view = render(<MemoizedMarkdown content="Ready" animateText />);

    view.rerender(<MemoizedMarkdown content="Ready $E = mc^2$" animateText />);

    expect(view.container.querySelector(".katex")).not.toBeNull();
    expect(view.container.querySelector(".katex [data-chat-text-motion]")).toBeNull();
  });
});
