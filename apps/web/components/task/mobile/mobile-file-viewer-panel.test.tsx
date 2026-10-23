/* eslint-disable max-lines -- this suite covers the complete mobile Markdown surface. */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useEffect, type ReactNode } from "react";
import type { SourcePosition, SourcePositionProps } from "@/components/editors/source-position";

const positionHarness = vi.hoisted(() => ({
  positions: {
    source: { nearestLine: 1 },
    edit: { nearestLine: 1 },
    preview: { nearestLine: 1 },
  } as Record<"source" | "edit" | "preview", SourcePosition>,
}));
function PositionBoundary({
  mode,
  onSourcePositionHandle,
  isActive = true,
}: SourcePositionProps & { mode: "source" | "edit" | "preview" }) {
  useEffect(() => {
    if (!onSourcePositionHandle || !isActive) return;
    onSourcePositionHandle({
      capture: () => positionHarness.positions[mode],
      restore: (position) => {
        positionHarness.positions[mode] = position;
      },
    });
    return () => onSourcePositionHandle(null);
  }, [mode, onSourcePositionHandle, isActive]);
  return null;
}

const getWebSocketClientMock = vi.hoisted(() => vi.fn(() => ({})));
const updateFileContentMock = vi.hoisted(() => vi.fn());
const MOBILE_SOURCE_CHANGE_LABEL = vi.hoisted(() => "Change mobile source");
const commentMocks = vi.hoisted(() => ({
  addComment: vi.fn(),
  selectComments: vi.fn(),
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
const MOBILE_EDIT_CONTENT = "# mobile edit";
const MOBILE_NEWER_EDIT_CONTENT = "# newer mobile edit";
const MOBILE_MARKDOWN_PATH = "README.md";
const MOBILE_MARKDOWN_CONTENT = "# README";
const MODE_TRIGGER_TEST_ID = "mobile-markdown-mode-trigger";
const HYBRID_EDITOR_TEST_ID = "mobile-hybrid-editor";
function selectMode(mode: "edit" | "preview" | "source") {
  fireEvent.keyDown(screen.getByTestId(MODE_TRIGGER_TEST_ID), { key: "Enter" });
  fireEvent.click(screen.getByTestId(`mobile-markdown-mode-${mode}`));
}
const SELECTED_ATTRIBUTE = "true";
const EDITABLE_ATTRIBUTE = "data-editable";

const htmlPreview = vi.hoisted(() => ({
  isPublishing: false,
  url: "http://api.test/port-proxy/session-1/43127/reports/index.html?v=1" as string | null,
  error: null,
  publish: vi
    .fn()
    .mockResolvedValue("http://api.test/port-proxy/session-1/43127/reports/index.html?v=1"),
  reset: vi.fn(),
}));

const state = {
  taskSessions: {
    items: {
      "session-1": {
        id: "session-1",
        task_id: "task-1",
        repository_id: "primary-repo",
        workspace_path: "/tmp/task-root",
        worktree_path: "/tmp/task-root/kandev",
      },
    },
  },
  tasks: { activeTaskId: "task-1" },
  kanban: { tasks: [] },
  kanbanMulti: { snapshots: {} },
  workspaces: { activeId: null },
  previewFeedback: {
    byTaskId: {},
    loadedByTaskId: {},
    loadingByTaskId: {},
    errorByTaskId: {},
  },
  connection: { status: "disconnected" },
};
const storeApi = { getState: () => state };
const REPORT_CONTENT = "<h1>Report</h1>";
const REPORT_PATH = "reports/index.html";
const FILE_CONTENT_TEST_ID = "file-content";

vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (_selector: unknown, fallback: unknown) => fallback,
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => storeApi,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: getWebSocketClientMock,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  updateFileContent: (...args: unknown[]) => updateFileContentMock(...args),
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

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  useExternalVcsFileStatus: () => ({ status: "untracked" }),
}));

vi.mock("../file-viewer-content", () => ({
  FileViewerContent: ({
    editable,
    onChange,
    onSourcePositionHandle,
    isActive,
  }: SourcePositionProps & {
    editable?: boolean;
    onChange?: (content: string) => void;
  }) => (
    <div data-testid={FILE_CONTENT_TEST_ID} data-editable={String(editable)}>
      <PositionBoundary
        mode="source"
        onSourcePositionHandle={onSourcePositionHandle}
        isActive={isActive}
      />
      <button type="button" onClick={() => onChange?.(MOBILE_EDIT_CONTENT)}>
        {MOBILE_SOURCE_CHANGE_LABEL}
      </button>
      <button type="button" onClick={() => onChange?.(MOBILE_NEWER_EDIT_CONTENT)}>
        Change mobile source again
      </button>
    </div>
  ),
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("@/components/editors/markdown/hybrid-markdown-editor", () => ({
  isHybridMarkdownSupported: () => true,
  HybridMarkdownEditor: ({
    baseline,
    comments,
    onComment,
    onChange,
    onSourceFallback,
    onOpenLink,
    onSourcePositionHandle,
    isActive,
  }: SourcePositionProps & {
    baseline?: string;
    comments?: readonly unknown[];
    onComment?: (comment: { text: string; start: number; endExclusive: number }) => void;
    onChange: (content: string) => void;
    onSourceFallback?: () => void;
    onOpenLink?: (url: string) => void;
  }) => (
    <div
      data-testid="mobile-hybrid-editor"
      data-baseline={baseline}
      className="h-full min-h-0 overflow-y-auto overscroll-contain"
    >
      <PositionBoundary
        mode="edit"
        onSourcePositionHandle={onSourcePositionHandle}
        isActive={isActive}
      />
      <span data-testid="mobile-hybrid-comment-count">{comments?.length ?? 0}</span>
      <button type="button" onClick={() => onChange("# hybrid mobile edit")}>
        Change mobile hybrid
      </button>
      <button
        type="button"
        onClick={() => onComment?.({ text: "Review mobile", start: 0, endExclusive: 8 })}
      >
        Add mobile hybrid comment
      </button>
      <button type="button" onClick={() => onSourceFallback?.()}>
        Fallback to source
      </button>
      <button type="button" onClick={() => onOpenLink?.("./guide.md")}>
        Open mobile Markdown link
      </button>
    </div>
  ),
}));
vi.mock("../markdown-preview-content", () => ({
  MarkdownPreviewContent: (props: SourcePositionProps) => (
    <span data-testid="markdown-preview">
      <PositionBoundary {...props} mode="preview" />
    </span>
  ),
}));
vi.mock("../html-preview-content", () => ({
  HtmlPreviewContent: ({ onTogglePreview }: { onTogglePreview: () => void }) => (
    <div data-testid="html-preview-frame">
      <button type="button" onClick={onTogglePreview}>
        Show code
      </button>
    </div>
  ),
}));
vi.mock("@/hooks/use-html-preview-publisher", () => ({
  useHtmlPreviewPublisher: () => htmlPreview,
  getHtmlPreviewPublishErrorKey: () => "task:htmlPreviewPublishFailed",
}));
vi.mock("../file-image-viewer", () => ({ FileImageViewer: () => null }));
vi.mock("../file-binary-viewer", () => ({
  FileBinaryViewer: ({ worktreePath }: { worktreePath?: string }) => (
    <span data-testid="binary-viewer" data-worktree-path={worktreePath} />
  ),
}));

import { MobileFileViewerPanel } from "./mobile-file-viewer-panel";

const MOBILE_SAVE_TEST_ID = "mobile-file-save";
const MOBILE_PREVIEW_TEST_ID = "markdown-preview";
const SAVED_HASH = "saved-hash";

afterEach(cleanup);

afterEach(() => {
  commentMocks.addComment.mockReset();
  commentMocks.comments = [];
});

beforeEach(() => {
  getWebSocketClientMock.mockReset();
  getWebSocketClientMock.mockReturnValue({});
  updateFileContentMock.mockReset();
});

describe("MobileFileViewerPanel workspace path", () => {
  it("uses the effective workspace path for binary file viewers", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: "dist/archive.zip",
          name: "archive.zip",
          content: "",
          originalContent: "",
          originalHash: "hash",
          isDirty: false,
          isBinary: true,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId("binary-viewer").getAttribute("data-worktree-path")).toBe(
      "/tmp/task-root",
    );
  });
});

// eslint-disable-next-line max-lines-per-function -- this fixture covers the complete mobile editor workflow.
describe("MobileFileViewerPanel Markdown editing", () => {
  it("opens a newly selected Markdown file in Preview mode", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(MODE_TRIGGER_TEST_ID).getAttribute("data-mode")).toBe("preview");
    expect(screen.queryByTestId(FILE_CONTENT_TEST_ID)).toBeNull();
  });

  it("switches between mobile Source and Edit while keeping changes in the file buffer", () => {
    const onFileChange = vi.fn();
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "source",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
        onFileChange={onFileChange}
      />,
    );

    selectMode("edit");
    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID).getAttribute("data-baseline")).toBeNull();
    expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID).className).toContain("overflow-y-auto");
    fireEvent.click(screen.getByRole("button", { name: "Change mobile hybrid" }));
    expect(onFileChange).toHaveBeenCalledWith("# hybrid mobile edit");
    expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(false);

    selectMode("preview");
    expect(screen.getByTestId("mobile-markdown-hybrid-editor-host").className).toBe("hidden");
    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBeTruthy();
  });

  it("keeps the mobile Preview surface and reading position across mode changes", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "preview",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );
    const preview = screen.getByTestId(MOBILE_PREVIEW_TEST_ID) as HTMLElement;
    preview.scrollTop = 160;

    selectMode("edit");
    selectMode("preview");

    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBe(preview);
    expect(preview.scrollTop).toBe(160);
  });

  it("maps existing comments and stores a submitted hybrid selection", () => {
    commentMocks.comments = [
      {
        id: "mobile-comment-1",
        source: "diff",
        sessionId: "session-1",
        filePath: MOBILE_MARKDOWN_PATH,
        startLine: 1,
        endLine: 1,
        side: "additions",
        codeContent: MOBILE_MARKDOWN_CONTENT,
        text: "Existing mobile review",
        createdAt: new Date().toISOString(),
        status: "pending",
      },
    ];
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "edit",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId("mobile-hybrid-comment-count").textContent).toBe("1");
    fireEvent.click(screen.getByRole("button", { name: "Add mobile hybrid comment" }));

    expect(commentMocks.addComment).toHaveBeenCalledWith(
      expect.objectContaining({
        filePath: MOBILE_MARKDOWN_PATH,
        sessionId: "session-1",
        startLine: 1,
        endLine: 1,
        codeContent: MOBILE_MARKDOWN_CONTENT,
        text: "Review mobile",
      }),
    );
  });

  it("routes Edit-mode Markdown links through the mobile file opener", () => {
    const onOpenFile = vi.fn();
    render(
      <MobileFileViewerPanel
        file={{
          path: "docs/readme.md",
          name: "readme.md",
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "edit",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
        onOpenFile={onOpenFile}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open mobile Markdown link" }));

    expect(onOpenFile).toHaveBeenCalledWith("docs/guide.md", undefined);
  });

  it("saves the canonical mobile buffer and clears the dirty state", async () => {
    updateFileContentMock.mockResolvedValue({
      path: MOBILE_MARKDOWN_PATH,
      success: true,
      new_hash: SAVED_HASH,
    });
    const onFileSaved = vi.fn();
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "source",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
        onFileSaved={onFileSaved}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: MOBILE_SOURCE_CHANGE_LABEL }));
    fireEvent.click(screen.getByTestId(MOBILE_SAVE_TEST_ID));

    await waitFor(() =>
      expect(updateFileContentMock).toHaveBeenCalledWith(
        {},
        "session-1",
        expect.objectContaining({
          path: MOBILE_MARKDOWN_PATH,
          originalHash: "hash",
          desiredContent: MOBILE_EDIT_CONTENT,
        }),
      ),
    );
    await waitFor(() =>
      expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(true),
    );
    expect(onFileSaved).toHaveBeenCalledWith({
      path: MOBILE_MARKDOWN_PATH,
      repo: undefined,
      sessionId: "session-1",
      content: MOBILE_EDIT_CONTENT,
      originalContent: MOBILE_EDIT_CONTENT,
      originalHash: SAVED_HASH,
    });
  });

  // @covers AC-UI-MARKDOWN-FILE-EDITING-002.14
  it("accepts a remote baseline equal to the local draft", async () => {
    const file = {
      path: MOBILE_MARKDOWN_PATH,
      name: MOBILE_MARKDOWN_PATH,
      content: MOBILE_MARKDOWN_CONTENT,
      originalContent: MOBILE_MARKDOWN_CONTENT,
      originalHash: "hash-A",
      isDirty: false,
      markdownMode: "source" as const,
    };
    const props = { sessionId: "session-1", onClose: vi.fn() };
    const view = render(<MobileFileViewerPanel {...props} file={file} />);
    fireEvent.click(screen.getByRole("button", { name: MOBILE_SOURCE_CHANGE_LABEL }));
    view.rerender(
      <MobileFileViewerPanel
        {...props}
        file={{
          ...file,
          content: MOBILE_EDIT_CONTENT,
          isDirty: true,
        }}
      />,
    );
    view.rerender(
      <MobileFileViewerPanel
        {...props}
        file={{
          ...file,
          content: MOBILE_EDIT_CONTENT,
          originalContent: MOBILE_EDIT_CONTENT,
          originalHash: "hash-B",
          isDirty: false,
        }}
      />,
    );
    expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Change mobile source again" }));
    updateFileContentMock.mockResolvedValue({ success: true, new_hash: SAVED_HASH });
    fireEvent.click(screen.getByTestId(MOBILE_SAVE_TEST_ID));
    await waitFor(() =>
      expect(updateFileContentMock).toHaveBeenCalledWith(
        {},
        "session-1",
        expect.objectContaining({
          originalHash: "hash-B",
          desiredContent: MOBILE_NEWER_EDIT_CONTENT,
        }),
      ),
    );
  });

  it("saves an editable mobile buffer from the platform keyboard shortcut", async () => {
    updateFileContentMock.mockResolvedValue({
      path: MOBILE_MARKDOWN_PATH,
      success: true,
      new_hash: SAVED_HASH,
    });
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "source",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: MOBILE_SOURCE_CHANGE_LABEL }));
    fireEvent.keyDown(screen.getByTestId("mobile-file-viewer-panel"), {
      key: "s",
      ctrlKey: true,
    });

    await waitFor(() =>
      expect(updateFileContentMock).toHaveBeenCalledWith(
        {},
        "session-1",
        expect.objectContaining({ desiredContent: MOBILE_EDIT_CONTENT }),
      ),
    );
  });

  it("preserves edits made after a mobile save starts", async () => {
    let resolveSave!: (value: { path: string; success: boolean; new_hash: string }) => void;
    updateFileContentMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSave = resolve;
        }),
    );
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "source",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: MOBILE_SOURCE_CHANGE_LABEL }));
    fireEvent.click(screen.getByTestId(MOBILE_SAVE_TEST_ID));
    await waitFor(() => expect(updateFileContentMock).toHaveBeenCalledOnce());

    fireEvent.click(screen.getByRole("button", { name: "Change mobile source again" }));
    resolveSave({ path: MOBILE_MARKDOWN_PATH, success: true, new_hash: SAVED_HASH });

    await waitFor(() =>
      expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(false),
    );

    updateFileContentMock.mockResolvedValue({
      path: MOBILE_MARKDOWN_PATH,
      success: true,
      new_hash: "newer-saved-hash",
    });
    fireEvent.click(screen.getByTestId(MOBILE_SAVE_TEST_ID));
    await waitFor(() =>
      expect(updateFileContentMock).toHaveBeenCalledWith(
        expect.anything(),
        "session-1",
        expect.objectContaining({
          desiredContent: MOBILE_NEWER_EDIT_CONTENT,
          originalHash: SAVED_HASH,
        }),
      ),
    );
  });

  it("does not apply a completed save to a newly selected file", async () => {
    let resolveSave!: (value: { path: string; success: boolean; new_hash: string }) => void;
    updateFileContentMock.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveSave = resolve;
        }),
    );
    const { rerender } = render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "source",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: MOBILE_SOURCE_CHANGE_LABEL }));
    fireEvent.click(screen.getByTestId(MOBILE_SAVE_TEST_ID));
    await waitFor(() => expect(updateFileContentMock).toHaveBeenCalledOnce());

    rerender(
      <MobileFileViewerPanel
        file={{
          path: "other.md",
          name: "other.md",
          content: "# other",
          originalContent: "# other",
          originalHash: "other-hash",
          isDirty: false,
          markdownMode: "source",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );
    resolveSave({ path: MOBILE_MARKDOWN_PATH, success: true, new_hash: SAVED_HASH });

    await waitFor(() =>
      expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(true),
    );
  });

  it("keeps Preview available for MDX but does not expose Edit", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: "README.mdx",
          name: "README.mdx",
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "preview",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId(MODE_TRIGGER_TEST_ID).getAttribute("data-mode")).toBe("preview");
    fireEvent.keyDown(screen.getByTestId(MODE_TRIGGER_TEST_ID), { key: "Enter" });
    expect(screen.getByTestId("mobile-markdown-mode-source")).toBeTruthy();
    expect(screen.queryByTestId("mobile-markdown-mode-edit")).toBeNull();
  });

  it("falls back to editable Source mode when the hybrid editor reports an error", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
          markdownMode: "edit",
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Fallback to source" }));
    expect(screen.getByTestId(FILE_CONTENT_TEST_ID).getAttribute(EDITABLE_ATTRIBUTE)).toBe(
      SELECTED_ATTRIBUTE,
    );
    expect(screen.getByTestId(MODE_TRIGGER_TEST_ID).getAttribute("data-mode")).toBe("source");
  });
});

describe("MobileFileViewerPanel external file action", () => {
  it("renders a touch-sized action scoped to the open file's repository", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: "src/new.ts",
          name: "new.ts",
          repo: "frontend",
          content: "",
          originalContent: "",
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: "src/new.ts",
      status: "untracked",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryName: "frontend",
      size: "touch",
    });
  });

  it("opens a Markdown file directly in preview mode when requested", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
        initialMarkdownPreview
      />,
    );

    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(FILE_CONTENT_TEST_ID)).toBeNull();
  });

  it("resets preview mode when the same path is opened from another repository", () => {
    const { rerender } = render(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          repo: "frontend",
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    selectMode("preview");
    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBeTruthy();

    rerender(
      <MobileFileViewerPanel
        file={{
          path: MOBILE_MARKDOWN_PATH,
          name: MOBILE_MARKDOWN_PATH,
          repo: "backend",
          content: MOBILE_MARKDOWN_CONTENT,
          originalContent: MOBILE_MARKDOWN_CONTENT,
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(FILE_CONTENT_TEST_ID)).toBeNull();
  });
});

describe("MobileFileViewerPanel HTML preview mode", () => {
  it("previews an HTML file and returns to the source viewer", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: REPORT_PATH,
          name: "index.html",
          content: REPORT_CONTENT,
          originalContent: REPORT_CONTENT,
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    const toggle = screen.getByTestId("html-preview-toggle");
    expect(toggle.className).toContain("h-11");
    expect(toggle.className).toContain("min-w-11");
    fireEvent.click(toggle);
    expect(htmlPreview.publish).toHaveBeenCalledWith({
      path: REPORT_PATH,
      repo: undefined,
      content: REPORT_CONTENT,
    });
    expect(screen.getByTestId("html-preview-frame")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Show code" }));
    expect(screen.getByTestId(FILE_CONTENT_TEST_ID)).toBeTruthy();
  });

  it("does not restore persisted in-place preview state for HTML", () => {
    render(
      <MobileFileViewerPanel
        file={{
          path: REPORT_PATH,
          name: "index.html",
          content: REPORT_CONTENT,
          originalContent: REPORT_CONTENT,
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
        initialRenderedPreview
      />,
    );

    expect(screen.getByTestId(FILE_CONTENT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("html-preview-frame")).toBeNull();
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
it("qualifies secondary-repository hybrid comments without the primary repository ID", () => {
  render(
    <MobileFileViewerPanel
      file={{
        path: "README.md",
        name: "README.md",
        repo: "secondary",
        content: "# README",
        originalContent: "# README",
        originalHash: "hash",
        isDirty: false,
        markdownMode: "edit",
      }}
      sessionId="session-1"
      onClose={vi.fn()}
    />,
  );
  expect(commentMocks.selectComments).toHaveBeenLastCalledWith(
    "session-1",
    "README.md",
    undefined,
    "secondary",
  );
  fireEvent.click(screen.getByRole("button", { name: "Add mobile hybrid comment" }));
  expect(commentMocks.addComment).toHaveBeenLastCalledWith(
    expect.objectContaining({ repositoryName: "secondary" }),
  );
  expect(commentMocks.addComment.mock.lastCall?.[0].repositoryId).toBeUndefined();
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.15
it("unmounts failed hybrid state and explicitly retries without overwriting mode preference", () => {
  const onModeChange = vi.fn();
  render(
    <MobileFileViewerPanel
      file={{
        path: "README.md",
        name: "README.md",
        content: "# README",
        originalContent: "# README",
        originalHash: "hash",
        isDirty: false,
        markdownMode: "edit",
      }}
      sessionId="session-1"
      onClose={vi.fn()}
      onModeChange={onModeChange}
    />,
  );
  const first = screen.getByTestId(HYBRID_EDITOR_TEST_ID);
  fireEvent.click(screen.getByRole("button", { name: "Fallback to source" }));
  expect(screen.queryByTestId(HYBRID_EDITOR_TEST_ID)).toBeNull();
  expect(onModeChange).not.toHaveBeenCalled();
  selectMode("edit");
  expect(screen.getByTestId(HYBRID_EDITOR_TEST_ID)).not.toBe(first);
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.4
it("transfers selection and nearest line between mobile Markdown modes", () => {
  const selected = { selection: { anchor: 7, active: 2 }, nearestLine: 1 };
  positionHarness.positions.source = selected;
  positionHarness.positions.edit = { nearestLine: 1 };
  positionHarness.positions.preview = { nearestLine: 1 };
  render(
    <MobileFileViewerPanel
      file={{
        path: "README.md",
        name: "README.md",
        content: "# README",
        originalContent: "# README",
        originalHash: "hash",
        isDirty: false,
        markdownMode: "source",
      }}
      sessionId="session-1"
      onClose={vi.fn()}
    />,
  );
  selectMode("preview");
  expect(positionHarness.positions.preview).toEqual(selected);
  selectMode("edit");
  expect(positionHarness.positions.edit).toEqual(selected);
  positionHarness.positions.edit = { selection: { anchor: 3, active: 6 }, nearestLine: 1 };
  selectMode("source");
  expect(positionHarness.positions.source).toEqual({
    selection: { anchor: 3, active: 6 },
    nearestLine: 1,
  });
});
