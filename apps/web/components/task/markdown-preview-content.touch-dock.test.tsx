import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

const STATE = vi.hoisted(() => ({ touch: false, openComposer: vi.fn() }));

vi.mock("@/hooks/domains/comments/use-markdown-preview-comments", () => ({
  useMarkdownPreviewComments: () => ({
    touch: STATE.touch,
    comments: [],
    commentView: null,
    currentSelection: {
      startLine: 1,
      endLine: 1,
      selectedText: "Install",
      position: { x: 40, y: 60 },
    },
    textSelection: null,
    dismissOverlays: vi.fn(),
    dismissForResize: vi.fn(),
    showCommentsForRange: vi.fn(),
    closeCommentView: vi.fn(),
    closeComposer: vi.fn(),
    openComposer: STATE.openComposer,
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
  STATE.touch = false;
  STATE.openComposer.mockClear();
});

function renderPreview() {
  render(
    <TooltipProvider>
      <MarkdownPreviewContent
        path="README.md"
        content="# Install"
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
}

describe("MarkdownPreviewContent selection comment trigger", () => {
  it("floats the comment button beside the selection with a fine pointer", () => {
    renderPreview();

    const button = screen.getByTestId("markdown-preview-comment-button");
    expect(button.classList.contains("floating-comment-btn")).toBe(true);
    expect(screen.queryByRole("toolbar", { name: "Selection actions" })).toBeNull();
  });

  it("keeps the fine-pointer comment button at the phone hit-target size", () => {
    renderPreview();

    const button = screen.getByTestId("markdown-preview-comment-button");
    expect(button.classList.contains("min-h-11")).toBe(true);
    expect(button.classList.contains("sm:min-h-8")).toBe(true);
  });

  it("docks the comment button in a toolbar on touch", () => {
    STATE.touch = true;
    renderPreview();

    const toolbar = screen.getByRole("toolbar", { name: "Selection actions" });
    const button = screen.getByTestId("markdown-preview-comment-button");
    expect(button.parentElement).toBe(toolbar);
    fireEvent.click(button);
    expect(STATE.openComposer).toHaveBeenCalledTimes(1);
  });
});
