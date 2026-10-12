import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import {
  resolveMarkdownDomSelection,
  SOURCE_END_ATTR,
  SOURCE_START_ATTR,
} from "@/lib/markdown/source-line-ranges";

const showCommentsForRange = vi.fn();

vi.mock("@/hooks/domains/comments/use-markdown-preview-comments", () => ({
  useMarkdownPreviewComments: () => ({
    comments: [
      { id: "single-line", startLine: 3, endLine: 3 },
      { id: "multiline", startLine: 7, endLine: 9 },
    ],
    commentView: null,
    currentSelection: null,
    textSelection: null,
    dismissOverlays: vi.fn(),
    showCommentsForRange,
    closeCommentView: vi.fn(),
    closeComposer: vi.fn(),
    openComposer: vi.fn(),
    removeComment: vi.fn(),
    submitAndRunComment: vi.fn(),
    submitComment: vi.fn(),
    updateComment: vi.fn(),
  }),
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => ({ status: "modified" }),
}));

import { MarkdownPreviewContent } from "./markdown-preview-content";

afterEach(() => {
  cleanup();
  showCommentsForRange.mockClear();
});

const content = [
  "Before formula",
  "",
  "$$\\frac{a}{b}$$",
  "",
  "Between formulas",
  "",
  "$$",
  "\\frac{c}{d}",
  "$$",
  "After formula",
].join("\n");

describe("MarkdownPreviewContent math source ranges", () => {
  it("keeps display formulas attached to source comments and selections", () => {
    render(
      <TooltipProvider>
        <MarkdownPreviewContent
          path="formula.md"
          content={content}
          sessionId="session-1"
          taskId="task-1"
          repositoryId="repo-1"
          repositoryName="frontend"
          enableComments
          showExternalVcsLink={false}
          onTogglePreview={vi.fn()}
        />
      </TooltipProvider>,
    );

    const preview = screen.getByTestId("markdown-preview");
    const displays = [...preview.querySelectorAll<HTMLElement>(".katex-display")];
    expect(displays).toHaveLength(2);

    const singleLineSource = displays[0].closest<HTMLElement>(`[${SOURCE_START_ATTR}]`);
    const multilineSource = displays[1].closest<HTMLElement>(`[${SOURCE_START_ATTR}]`);
    expect(singleLineSource?.getAttribute(SOURCE_START_ATTR)).toBe("3");
    expect(singleLineSource?.getAttribute(SOURCE_END_ATTR)).toBe("3");
    expect(multilineSource?.getAttribute(SOURCE_START_ATTR)).toBe("7");
    expect(multilineSource?.getAttribute(SOURCE_END_ATTR)).toBe("9");

    const root = preview.querySelector<HTMLElement>(".markdown-body");
    expect(root).not.toBeNull();
    expect(root?.querySelectorAll('[data-testid="markdown-preview-comment-badge"]')).toHaveLength(
      2,
    );
    const range = document.createRange();
    range.selectNodeContents(multilineSource!);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);

    expect(resolveMarkdownDomSelection(root!, content, selection)).toMatchObject({
      startLine: 7,
      endLine: 9,
    });

    selection.removeAllRanges();
    fireEvent.click(multilineSource!, { clientX: 10, clientY: 20 });
    expect(showCommentsForRange).toHaveBeenCalledWith(
      { startLine: 7, endLine: 9 },
      { x: 10, y: 20 },
    );
  });
});
