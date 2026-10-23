import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useEffect, useState, type ReactNode } from "react";
import type { SourcePosition, SourcePositionProps } from "@/components/editors/source-position";
import type { MarkdownFileMode } from "./markdown-file-mode";
const positionHarness = vi.hoisted(() => ({
  positions: {
    source: { nearestLine: 1 },
    edit: { nearestLine: 1 },
    preview: { nearestLine: 1 },
  } as Record<MarkdownFileMode, SourcePosition>,
}));
function PositionBoundary({
  mode,
  onSourcePositionHandle,
  isActive = true,
}: SourcePositionProps & { mode: MarkdownFileMode }) {
  useEffect(() => {
    if (!isActive || !onSourcePositionHandle) return;
    onSourcePositionHandle({
      capture: () => positionHarness.positions[mode],
      restore: (position) => {
        positionHarness.positions[mode] = position;
      },
    });
    return () => onSourcePositionHandle(null);
  }, [mode, isActive, onSourcePositionHandle]);
  return null;
}
import {
  MarkdownFileEditor,
  sourceLineEndOffset,
  sourceLinesAtOffsets,
  sourceOffsetAtLine,
} from "./markdown-file-editor";

const MARKDOWN_PREVIEW_CONTENT_TEST_ID = vi.hoisted(() => "markdown-preview-content");

const commentMocks = vi.hoisted(() => ({
  addComment: vi.fn(),
  selectComments: vi.fn(),
  hybridAvailable: true,
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
  useDiffFileComments: (...args: unknown[]) => {
    commentMocks.selectComments(...args);
    return commentMocks.comments;
  },
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
    onSourcePositionHandle,
    isActive,
  }: SourcePositionProps & {
    markdownPreview?: boolean;
    toolbarModeControl?: ReactNode;
    onChange: (value: string) => void;
    onSave: () => void;
  }) => (
    <div data-testid="source-editor" data-markdown-preview={String(markdownPreview)}>
      <PositionBoundary
        mode="source"
        isActive={isActive}
        onSourcePositionHandle={onSourcePositionHandle}
      />
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
    onSourcePositionHandle,
    isActive,
  }: SourcePositionProps & {
    content: string;
    toolbarModeControl?: ReactNode;
    showToolbar?: boolean;
  }) => (
    <div data-testid={MARKDOWN_PREVIEW_CONTENT_TEST_ID}>
      <PositionBoundary
        mode="preview"
        isActive={isActive}
        onSourcePositionHandle={onSourcePositionHandle}
      />
      {showToolbar && <div data-testid="preview-toolbar-mode-control">{toolbarModeControl}</div>}
      {content}
    </div>
  ),
}));

vi.mock("@/components/editors/markdown/hybrid-markdown-editor", () => ({
  isHybridMarkdownSupported: () => commentMocks.hybridAvailable,
  HybridMarkdownEditor: ({
    content,
    baseline,
    onChange,
    comments,
    onComment,
    onSourceFallback,
    onSourcePositionHandle,
    isActive,
  }: SourcePositionProps & {
    content: string;
    baseline?: string;
    onChange: (value: string) => void;
    comments?: readonly unknown[];
    onComment?: (comment: { text: string; start: number; endExclusive: number }) => void;
    onSourceFallback?: () => void;
  }) => (
    <div
      data-testid="hybrid-editor"
      data-content={content}
      data-baseline={baseline}
      data-comment-count={String(comments?.length ?? 0)}
    >
      <PositionBoundary
        mode="edit"
        isActive={isActive}
        onSourcePositionHandle={onSourcePositionHandle}
      />
      <button type="button" onClick={onSourceFallback}>
        Fail hybrid
      </button>
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
  commentMocks.hybridAvailable = true;
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

describe("Markdown comparison presentation", () => {
  it("does not turn the ordinary saved buffer into a stacked comparison", () => {
    render(
      <MarkdownFileEditor
        {...baseProps}
        mode="edit"
        content="# locally edited"
        originalContent="# saved source"
        isDirty
      />,
    );

    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID).getAttribute("data-baseline")).toBeNull();
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

    expect(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID).className).toContain("h-5");
    expect(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID).className).not.toContain("h-6");
    expect(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID).className).not.toContain("h-8");
    expect(screen.getByTestId("markdown-file-delete").className).toContain("h-6");
    expect(screen.getByTestId("markdown-file-save").className).toContain("h-6");
    expect(screen.getByTestId("markdown-file-save").textContent).toMatch(
      /Save\s*\((?:Ctrl|⌘)\+S\)/,
    );
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

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
describe("Markdown repository-qualified comments", () => {
  it("reads and writes the selected secondary repository without the primary ID", () => {
    render(
      <MarkdownFileEditor
        {...baseProps}
        repo="secondary"
        repositoryId={undefined}
        mode="edit"
        enableComments
      />,
    );
    expect(commentMocks.selectComments).toHaveBeenLastCalledWith(
      "session-1",
      "README.md",
      undefined,
      "secondary",
    );
    fireEvent.click(screen.getByRole("button", { name: "Add hybrid comment" }));
    expect(commentMocks.addComment).toHaveBeenLastCalledWith(
      expect.objectContaining({ repositoryName: "secondary" }),
    );
    expect(commentMocks.addComment.mock.lastCall?.[0].repositoryId).toBeUndefined();
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.15
describe("Markdown hybrid recovery", () => {
  it("discards a failed retained editor and retries with the exact current draft", () => {
    const onModeChange = vi.fn();
    const { rerender } = render(
      <MarkdownFileEditor {...baseProps} mode="edit" onModeChange={onModeChange} />,
    );
    const first = screen.getByTestId(HYBRID_EDITOR_TEST_ID);
    fireEvent.click(screen.getByRole("button", { name: "Fail hybrid" }));
    expect(screen.queryByTestId(HYBRID_EDITOR_TEST_ID)).toBeNull();
    expect(screen.getByTestId("source-editor")).toBeTruthy();
    expect(onModeChange).not.toHaveBeenCalled();
    rerender(
      <MarkdownFileEditor
        {...baseProps}
        mode="edit"
        content={"# exact retry draft\r\n"}
        onModeChange={onModeChange}
      />,
    );
    fireEvent.click(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID));
    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID)).not.toBe(first);
    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID).getAttribute("data-content")).toBe(
      "# exact retry draft\r\n",
    );
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-001.10
it("keeps restored Edit preference intact while showing Source without browser capability", () => {
  commentMocks.hybridAvailable = false;
  const onModeChange = vi.fn();
  const onChange = vi.fn();
  const onSave = vi.fn();
  render(
    <MarkdownFileEditor
      {...baseProps}
      mode="edit"
      content={"# preserved\r\n"}
      onModeChange={onModeChange}
      onChange={onChange}
      onSave={onSave}
    />,
  );
  expect(screen.getByTestId("source-editor")).toBeTruthy();
  expect((screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByTestId(MARKDOWN_EDIT_MODE_TEST_ID).getAttribute("title")).toBeTruthy();
  expect(screen.getByRole("status")).toBeTruthy();
  expect(onModeChange).not.toHaveBeenCalled();
  expect(onChange).not.toHaveBeenCalled();
  expect(onSave).not.toHaveBeenCalled();
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
it("scopes primary repository hybrid reads with the empty repository name", () => {
  render(<MarkdownFileEditor {...baseProps} repo={undefined} mode="edit" enableComments />);
  expect(commentMocks.selectComments).toHaveBeenLastCalledWith(
    "session-1",
    "README.md",
    "repo-1",
    "",
  );
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-001.5
it("transfers selection and nearest line between desktop Markdown modes", () => {
  const selected = { selection: { anchor: 7, active: 2 }, nearestLine: 1 };
  positionHarness.positions.source = selected;
  positionHarness.positions.preview = { nearestLine: 1 };
  positionHarness.positions.edit = { nearestLine: 1 };
  function ControlledEditor() {
    const [mode, setMode] = useState<MarkdownFileMode>("source");
    return <MarkdownFileEditor {...baseProps} mode={mode} onModeChange={setMode} />;
  }
  render(<ControlledEditor />);
  fireEvent.click(screen.getByTestId("markdown-mode-preview"));
  expect(positionHarness.positions.preview).toEqual(selected);
  fireEvent.click(screen.getByTestId("markdown-mode-edit"));
  expect(positionHarness.positions.edit).toEqual(selected);
  positionHarness.positions.edit = { selection: { anchor: 3, active: 6 }, nearestLine: 1 };
  fireEvent.click(screen.getByTestId("markdown-mode-source"));
  expect(positionHarness.positions.source).toEqual({
    selection: { anchor: 3, active: 6 },
    nearestLine: 1,
  });
});
