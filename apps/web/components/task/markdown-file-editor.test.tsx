import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import {
  MarkdownFileEditor,
  sourceLineEndOffset,
  sourceLinesAtOffsets,
  sourceOffsetAtLine,
} from "./markdown-file-editor";

const MARKDOWN_PREVIEW_CONTENT_TEST_ID = vi.hoisted(() => "markdown-preview-content");

const commentMocks = vi.hoisted(() => ({
  addComment: vi.fn(),
  comments: [] as Array<{
    id: string;
    source: "diff";
    sessionId: string;
    filePath: string;
    startLine: number;
    endLine: number;
    side: "additions";
    codeContent: string;
    text: string;
    createdAt: string;
    status: "pending";
  }>,
}));

vi.mock("@/hooks/domains/comments/use-diff-comments", () => ({
  useDiffFileComments: () => commentMocks.comments,
}));

vi.mock("@/lib/state/slices/comments", () => ({
  useCommentsStore: (
    selector: (state: { addComment: typeof commentMocks.addComment }) => unknown,
  ) => selector({ addComment: commentMocks.addComment }),
}));

vi.mock("./file-editor-content", () => ({
  FileEditorContent: ({
    markdownPreview,
    toolbarModeControl,
    onChange,
    onSave,
  }: {
    markdownPreview?: boolean;
    toolbarModeControl?: ReactNode;
    onChange: (value: string) => void;
    onSave: () => void;
  }) => (
    <div data-testid="source-editor" data-markdown-preview={String(markdownPreview)}>
      <div data-testid="source-toolbar-mode-control">{toolbarModeControl}</div>
      <button type="button" onClick={() => onChange("# edited")}>
        Change
      </button>
      <button type="button" onClick={onSave}>
        Save source
      </button>
    </div>
  ),
}));

vi.mock("./markdown-preview-content", () => ({
  MarkdownPreviewContent: ({
    content,
    toolbarModeControl,
    showToolbar = true,
  }: {
    content: string;
    toolbarModeControl?: ReactNode;
    showToolbar?: boolean;
  }) => (
    <div data-testid={MARKDOWN_PREVIEW_CONTENT_TEST_ID}>
      {showToolbar && <div data-testid="preview-toolbar-mode-control">{toolbarModeControl}</div>}
      {content}
    </div>
  ),
}));

vi.mock("@/components/editors/markdown/hybrid-markdown-editor", () => ({
  HybridMarkdownEditor: ({
    content,
    onChange,
    comments,
    onComment,
  }: {
    content: string;
    onChange: (value: string) => void;
    comments?: readonly unknown[];
    onComment?: (comment: { text: string; start: number; endExclusive: number }) => void;
  }) => (
    <div
      data-testid="hybrid-editor"
      data-content={content}
      data-comment-count={String(comments?.length ?? 0)}
    >
      <button type="button" onClick={() => onChange("# hybrid edit")}>
        Change hybrid
      </button>
      <button
        type="button"
        onClick={() => onComment?.({ text: "Review this", start: 0, endExclusive: 8 })}
      >
        Add hybrid comment
      </button>
    </div>
  ),
}));

vi.mock("@/components/task/file-viewer-header", () => ({
  FileViewerExternalLink: () => <span data-testid="external-link" />,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const HYBRID_EDITOR_TEST_ID = "hybrid-editor";
const MARKDOWN_EDIT_MODE_TEST_ID = "markdown-mode-edit";

afterEach(() => {
  cleanup();
  commentMocks.addComment.mockReset();
  commentMocks.comments = [];
});

const baseProps = {
  path: "README.md",
  content: "# source",
  originalContent: "# source",
  isDirty: false,
  isSaving: false,
  sessionId: "session-1",
  taskId: "task-1",
  repositoryId: "repo-1",
  worktreePath: "/tmp/worktree",
  repo: "frontend",
  onModeChange: vi.fn(),
  onChange: vi.fn(),
  onSave: vi.fn(),
  onReloadFromAgent: vi.fn(),
  onDelete: vi.fn(),
};

describe("Markdown source comment offsets", () => {
  it("maps line ranges and offsets without losing CRLF boundaries", () => {
    const content = "# first\r\n\r\nsecond\r\n";

    expect(sourceOffsetAtLine(content, 2)).toBe(9);
    expect(sourceLineEndOffset(content, 1)).toBe(7);
    expect(sourceLinesAtOffsets(content, 9, 15)).toEqual({
      startLine: 2,
      endLine: 3,
      selectedText: "\r\nseco",
    });
  });
});

describe("MarkdownFileEditor", () => {
  it("renders Preview and changes mode through one visible mode control", () => {
    render(<MarkdownFileEditor {...baseProps} mode="preview" />);

    const preview = screen.getByTestId(MARKDOWN_PREVIEW_CONTENT_TEST_ID);
    expect(preview).toBeTruthy();
    expect(screen.queryByTestId(HYBRID_EDITOR_TEST_ID)).toBeNull();
    expect(within(preview).getAllByTestId(MARKDOWN_EDIT_MODE_TEST_ID)).toHaveLength(1);
    fireEvent.click(within(preview).getByTestId(MARKDOWN_EDIT_MODE_TEST_ID));
    expect(baseProps.onModeChange).toHaveBeenCalledWith("edit");
  });

  it("renders the hybrid editor in Edit mode and forwards source changes", () => {
    const onChange = vi.fn();
    render(<MarkdownFileEditor {...baseProps} mode="edit" onChange={onChange} />);

    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID).getAttribute("data-content")).toBe("# source");
    fireEvent.click(screen.getByRole("button", { name: "Change hybrid" }));
    expect(onChange).toHaveBeenCalledWith("# hybrid edit");
    expect((screen.getByTestId("markdown-file-save") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("markdown-hybrid-editor-host").className).toContain(
      "overflow-hidden",
    );
  });

  it("keeps the hybrid editor mounted while switching to another presentation", () => {
    const { rerender } = render(<MarkdownFileEditor {...baseProps} mode="edit" />);
    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID)).toBeTruthy();

    rerender(<MarkdownFileEditor {...baseProps} mode="preview" />);

    expect(screen.getByTestId("markdown-hybrid-editor-host").className).toContain("hidden");
    expect(screen.getByTestId(MARKDOWN_PREVIEW_CONTENT_TEST_ID)).toBeTruthy();
  });

  it("keeps the Preview surface and its reading position mounted across mode changes", () => {
    const { rerender } = render(<MarkdownFileEditor {...baseProps} mode="preview" />);
    const preview = screen.getByTestId(MARKDOWN_PREVIEW_CONTENT_TEST_ID) as HTMLElement;
    preview.scrollTop = 160;

    rerender(<MarkdownFileEditor {...baseProps} mode="edit" />);
    rerender(<MarkdownFileEditor {...baseProps} mode="preview" />);

    expect(screen.getByTestId(MARKDOWN_PREVIEW_CONTENT_TEST_ID)).toBe(preview);
    expect(preview.scrollTop).toBe(160);
  });

  it("passes existing comments to Edit mode and stores new source ranges", () => {
    commentMocks.comments = [
      {
        id: "comment-1",
        source: "diff",
        sessionId: "session-1",
        filePath: "README.md",
        startLine: 1,
        endLine: 1,
        side: "additions",
        codeContent: "# source",
        text: "Existing review",
        createdAt: new Date().toISOString(),
        status: "pending",
      },
    ];
    render(<MarkdownFileEditor {...baseProps} mode="edit" enableComments />);

    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID).getAttribute("data-comment-count")).toBe("1");
    fireEvent.click(screen.getByRole("button", { name: "Add hybrid comment" }));

    expect(commentMocks.addComment).toHaveBeenCalledWith(
      expect.objectContaining({
        source: "diff",
        filePath: "README.md",
        startLine: 1,
        endLine: 1,
        text: "Review this",
      }),
    );
  });

  it("uses the existing source editor and keeps Save available in Source mode", () => {
    render(<MarkdownFileEditor {...baseProps} mode="source" isDirty />);

    const sourceEditor = screen.getByTestId("source-editor");
    expect(sourceEditor.getAttribute("data-markdown-preview")).toBe("false");
    expect(
      within(sourceEditor).getByTestId("markdown-mode-source").getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.queryByTestId("markdown-mode-toolbar")).toBeNull();
  });

  it("uses compact desktop mode buttons that match the existing toolbar controls", () => {
    render(<MarkdownFileEditor {...baseProps} mode="edit" />);

    expect(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID).className).toContain("h-8");
    expect(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID).className).not.toContain("h-9");
  });

  it("leaves Source-mode keyboard save to the source editor", () => {
    const onSave = vi.fn();
    render(<MarkdownFileEditor {...baseProps} mode="source" onSave={onSave} />);

    fireEvent.keyDown(screen.getByTestId("markdown-file-editor"), { key: "s", ctrlKey: true });

    expect(onSave).not.toHaveBeenCalled();
  });

  it("omits Edit for MDX while keeping Preview and Source", () => {
    render(<MarkdownFileEditor {...baseProps} path="README.mdx" mode="preview" />);

    expect(screen.queryByTestId(MARKDOWN_EDIT_MODE_TEST_ID)).toBeNull();
    expect(screen.getByTestId("markdown-mode-preview")).toBeTruthy();
    expect(screen.getByTestId("markdown-mode-source")).toBeTruthy();
  });
});
