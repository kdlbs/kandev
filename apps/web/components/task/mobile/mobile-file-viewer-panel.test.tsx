import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

const getWebSocketClientMock = vi.hoisted(() => vi.fn(() => ({})));
const updateFileContentMock = vi.hoisted(() => vi.fn());
const MOBILE_EDIT_CONTENT = "# mobile edit";
const MOBILE_NEWER_EDIT_CONTENT = "# newer mobile edit";
const MOBILE_MARKDOWN_PATH = "README.md";
const MOBILE_MARKDOWN_CONTENT = "# README";
const TRUE_VALUE = true;
const SELECTED_ATTRIBUTE = String(TRUE_VALUE);
const EDITABLE_ATTRIBUTE = "data-editable";
const FILE_CONTENT_TEST_ID = "file-content";
const REPORT_PATH = "reports/index.html";
const REPORT_CONTENT = "<h1>Report</h1>";

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
const README_CONTENT = "# README";
const REPORT_CONTENT = "<h1>Report</h1>";
const README_PATH = "README.md";
const REPORT_PATH = "reports/index.html";
const FILE_CONTENT_TEST_ID = "file-content";

vi.mock("@/components/state-provider", () => ({
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
        Change mobile source
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
  HybridMarkdownEditor: ({
    onChange,
    onSourceFallback,
  }: {
    onChange: (content: string) => void;
    onSourceFallback?: () => void;
  }) => (
    <div data-testid="mobile-hybrid-editor">
      <button type="button" onClick={() => onChange("# hybrid mobile edit")}>
        Change mobile hybrid
      </button>
      <button type="button" onClick={() => onSourceFallback?.()}>
        Fallback to source
      </button>
    </div>
  ),
}));
vi.mock("../markdown-preview-content", () => ({
  MarkdownPreviewContent: () => <span data-testid="markdown-preview" />,
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
  it("opens a Markdown file in Source mode with an editable mobile surface", () => {
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

    expect(screen.getByTestId(FILE_CONTENT_TEST_ID).getAttribute(EDITABLE_ATTRIBUTE)).toBe(
      SELECTED_ATTRIBUTE,
    );
    expect(screen.getByTestId("mobile-markdown-mode-source")).toBeTruthy();
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
    fireEvent.click(screen.getByRole("button", { name: "Change mobile hybrid" }));
    expect(onFileChange).toHaveBeenCalledWith("# hybrid mobile edit");
    expect((screen.getByTestId(MOBILE_SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByTestId("mobile-markdown-mode-preview"));
    expect(screen.getByTestId("mobile-markdown-hybrid-editor-host").className).toBe("hidden");
    expect(screen.getByTestId(MOBILE_PREVIEW_TEST_ID)).toBeTruthy();
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

    fireEvent.click(screen.getByRole("button", { name: "Change mobile source" }));
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

    fireEvent.click(screen.getByRole("button", { name: "Change mobile source" }));
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
    fireEvent.click(screen.getByRole("button", { name: "Change mobile source" }));
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

    expect(screen.getByTestId("mobile-markdown-mode-preview")).toBeTruthy();
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

    fireEvent.click(screen.getByTestId("mobile-markdown-mode-preview"));
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

    expect(screen.getByTestId(FILE_CONTENT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(MOBILE_PREVIEW_TEST_ID)).toBeNull();
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
