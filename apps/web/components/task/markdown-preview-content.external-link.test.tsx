import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  useExternalVcsFileStatus: () => ({ status: "modified" }),
}));

vi.mock("@/hooks/domains/comments/use-markdown-preview-comments", () => ({
  useMarkdownPreviewComments: () => ({
    comments: [],
    commentView: null,
    currentSelection: null,
    textSelection: null,
    dismissOverlays: vi.fn(),
    showCommentsForRange: vi.fn(),
    closeCommentView: vi.fn(),
    closeComposer: vi.fn(),
    openComposer: vi.fn(),
    removeComment: vi.fn(),
    submitAndRunComment: vi.fn(),
    submitComment: vi.fn(),
    updateComment: vi.fn(),
  }),
}));

import { MarkdownPreviewContent } from "./markdown-preview-content";

afterEach(cleanup);

describe("MarkdownPreviewContent external file action", () => {
  it("keeps the desktop external action available while previewing", () => {
    render(
      <TooltipProvider>
        <MarkdownPreviewContent
          path="README.md"
          content="# Hello"
          sessionId="session-1"
          taskId="task-1"
          repositoryId="repo-1"
          repositoryName="frontend"
          onTogglePreview={vi.fn()}
        />
      </TooltipProvider>,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: "README.md",
      status: "modified",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryName: "frontend",
      size: "sm",
    });
  });
});
