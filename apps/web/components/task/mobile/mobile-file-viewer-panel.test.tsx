/* eslint-disable max-lines -- this suite covers the complete mobile Markdown surface. */

import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getWebSocketClientMock = vi.hoisted(() => vi.fn(() => ({})));
const updateFileContentMock = vi.hoisted(() => vi.fn());
const MOBILE_SOURCE_CHANGE_LABEL = vi.hoisted(() => "Change mobile source");
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
const MOBILE_EDIT_CONTENT = "# mobile edit";
const MOBILE_NEWER_EDIT_CONTENT = "# newer mobile edit";
const MOBILE_MARKDOWN_PATH = "README.md";
const MOBILE_MARKDOWN_CONTENT = "# README";
const MOBILE_MARKDOWN_PREVIEW_MODE_TEST_ID = "mobile-markdown-mode-preview";
const TRUE_VALUE = true;
const SELECTED_ATTRIBUTE = String(TRUE_VALUE);
const EDITABLE_ATTRIBUTE = "data-editable";
const FILE_CONTENT_TEST_ID = "file-content";

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
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
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
  useDiffFileComments: () => commentMocks.comments,
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
  }: {
    editable?: boolean;
    onChange?: (content: string) => void;
  }) => (
    <div data-testid="file-content" data-editable={String(editable)}>
      <button type="button" onClick={() => onChange?.(MOBILE_EDIT_CONTENT)}>
        {MOBILE_SOURCE_CHANGE_LABEL}
      </button>
      <button type="button" onClick={() => onChange?.(MOBILE_NEWER_EDIT_CONTENT)}>
        Change mobile source again
      </button>
    </div>
  ),
}));
vi.mock("@/components/editors/markdown/hybrid-markdown-editor", () => ({
  HybridMarkdownEditor: ({
    baseline,
    comments,
    onComment,
    onChange,
    onSourceFallback,
    onOpenLink,
  }: {
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
  MarkdownPreviewContent: () => <span data-testid="markdown-preview" />,
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
          isBinary: TRUE_VALUE,
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
    expect(screen.getByTestId(MOBILE_MARKDOWN_PREVIEW_MODE_TEST_ID)).toBeTruthy();
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

    fireEvent.click(screen.getByTestId("mobile-markdown-mode-edit"));
    expect(screen.getByTestId("mobile-hybrid-editor")).toBeTruthy();
    expect(screen.getByTestId("mobile-hybrid-editor").getAttribute("data-baseline")).toBeNull();
    expect(screen.getByTestId("mobile-hybrid-editor").className).toContain("overflow-y-auto");
    fireEvent.click(screen.getByRole("button", { name: "Change mobile hybrid" }));
    expect(onFileChange).toHaveBeenCalledWith("# hybrid mobile edit");
    expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(screen.getByTestId(MOBILE_MARKDOWN_PREVIEW_MODE_TEST_ID));
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

    fireEvent.click(screen.getByTestId("mobile-markdown-mode-edit"));
    fireEvent.click(screen.getByTestId(MOBILE_MARKDOWN_PREVIEW_MODE_TEST_ID));

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
      success: TRUE_VALUE,
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
      expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(
        TRUE_VALUE,
      ),
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

  it("saves an editable mobile buffer from the platform keyboard shortcut", async () => {
    updateFileContentMock.mockResolvedValue({
      path: MOBILE_MARKDOWN_PATH,
      success: TRUE_VALUE,
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

    expect(screen.getByTestId(MOBILE_MARKDOWN_PREVIEW_MODE_TEST_ID)).toBeTruthy();
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
    expect(screen.getByTestId("mobile-markdown-mode-source").getAttribute("aria-pressed")).toBe(
      SELECTED_ATTRIBUTE,
    );
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

    fireEvent.click(screen.getByTestId(MOBILE_MARKDOWN_PREVIEW_MODE_TEST_ID));
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
