import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { createRef, type ReactNode } from "react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { reviewFileKey, type ReviewFile } from "./types";

const mocks = vi.hoisted(() => ({
  isMobile: false,
  requestFileContent: vi.fn(),
}));

const publisher = vi.hoisted(() => {
  const state = {
    url: null as string | null,
    error: null as "publish-failed" | "session-unavailable" | "too-large" | null,
    isPublishing: false,
  };
  return {
    state,
    publish: vi.fn(),
    reset: vi.fn(() => {
      state.url = null;
      state.error = null;
    }),
  };
});

const HTML_PATH = "site/index.html";
const SOURCE_COPY = "Current workspace file";
const TRUST_COPY = "Previewing runs workspace code";
const REPO_NAME = "frontend";
const PREVIEW_HTML_LABEL = "Preview HTML";
const WORKSPACE_CONTENT = "<html>Workspace page</html>";
const PREVIEW_URL = "http://api.test/port-proxy/session-1/43127/site/index.html?v=1";

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ id: "ws" }) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: mocks.requestFileContent,
  updateFileContent: vi.fn(),
}));

vi.mock("@/hooks/use-html-preview-publisher", () => ({
  useHtmlPreviewPublisher: () => ({
    url: publisher.state.url,
    error: publisher.state.error,
    isPublishing: publisher.state.isPublishing,
    publish: publisher.publish,
    reset: publisher.reset,
  }),
}));

vi.mock("@/hooks/use-preview-capture", () => ({
  usePreviewCapture: () => ({
    items: [],
    snapshot: { task_id: "task-1", revision: 0, items: [] },
    mode: null,
    draft: null,
    draftComment: "",
    setDraftComment: vi.fn(),
    candidateLabel: null,
    captureError: null,
    isRasterizing: false,
    isUploading: false,
    isMutating: false,
    mutationError: null,
    startCapture: vi.fn(),
    cancelCapture: vi.fn(),
    discardDraft: vi.fn(),
    saveDraft: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
    handleIframeLoad: vi.fn(),
  }),
}));

const mocksForLayout = mocks;

// FileDiffViewer is heavy and irrelevant to the row-body replacement test.
vi.mock("@/components/diff", () => ({
  FileDiffViewer: ({ filePath }: { filePath: string }) => (
    <div data-testid="diff-stub">{filePath}</div>
  ),
  DiffErrorBoundary: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/task/markdown-preview-content", () => ({
  MarkdownPreviewRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => null,
  FileActionsMenuItems: () => null,
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  ExternalVcsFileMenuItem: () => null,
  useExternalVcsFileStatus: () => null,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocksForLayout.isMobile,
    isFinePointer: true,
  }),
}));

vi.mock("@/hooks/use-global-view-mode", () => ({
  useGlobalViewMode: () => ["unified", vi.fn()],
}));

vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { openBrowserPanel: () => void }) => unknown) =>
    selector({ openBrowserPanel: vi.fn() }),
}));

import { ReviewDiffList } from "./review-diff-list";

const SESSION_ID = "session-1";

function reviewFile(path: string, repo?: string): ReviewFile {
  return {
    path,
    diff: "@@ -1 +1 @@\n-a\n+b\n",
    status: "modified",
    additions: 1,
    deletions: 1,
    staged: false,
    source: "uncommitted",
    repository_name: repo,
  };
}

const baseProps = {
  reviewedFiles: new Set<string>(),
  staleFiles: new Set<string>(),
  sessionId: SESSION_ID,
  autoMarkOnScroll: false,
  wordWrap: false,
  enableWalkthroughAnnotations: true,
  selectedFile: null,
  onToggleReviewed: () => undefined,
  onDiscard: () => undefined,
  fileRefs: new Map<string, React.RefObject<HTMLDivElement | null>>(),
};

function renderList(files: ReviewFile[], reviewedFiles = new Set<string>(), selectedFile?: string) {
  const refs = new Map(files.map((file) => [reviewFileKey(file), createRef<HTMLDivElement>()]));
  render(
    <TooltipProvider>
      <ReviewDiffList
        {...baseProps}
        files={files}
        reviewedFiles={reviewedFiles}
        selectedFile={selectedFile ?? (files[0] ? reviewFileKey(files[0]) : null)}
        fileRefs={refs}
      />
    </TooltipProvider>,
  );
}

function headerShowDiff() {
  return within(screen.getByTestId("review-file-header")).getByRole("button", {
    name: "Show diff",
  });
}

function workspaceContent(content: string, isBinary = false) {
  return { path: HTML_PATH, content, size: content.length, is_binary: isBinary };
}

beforeEach(() => {
  publisher.publish.mockReset();
  publisher.publish.mockImplementation(async () => {
    publisher.state.url = PREVIEW_URL;
    return PREVIEW_URL;
  });
  publisher.reset.mockClear();
  publisher.state.url = null;
  publisher.state.error = null;
  publisher.state.isPublishing = false;
  mocks.requestFileContent.mockReset();
});

afterEach(() => {
  cleanup();
  mocksForLayout.isMobile = false;
});

describe("ReviewDiffList HTML preview", () => {
  it("replaces only the selected HTML row body and returns to its diff", async () => {
    mocks.requestFileContent.mockResolvedValue(workspaceContent(WORKSPACE_CONTENT));
    const htmlFile = reviewFile(HTML_PATH, REPO_NAME);
    renderList([htmlFile], new Set([reviewFileKey(htmlFile)]));

    const wrap = screen.getByRole("button", { name: "Toggle word wrap" });
    fireEvent.click(wrap);
    expect(wrap.getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));

    await waitFor(() => expect(screen.getByTestId("review-html-diff-preview")).toBeTruthy());
    expect(screen.getByTestId("review-html-preview-source").textContent).toContain(SOURCE_COPY);
    expect(screen.getByTestId("html-preview-trust-warning").textContent).toContain(TRUST_COPY);
    expect(screen.getByTestId("html-preview-frame").getAttribute("src")).toBe(PREVIEW_URL);
    expect(screen.queryByTestId("diff-stub")).toBeNull();
    expect(screen.getByRole("checkbox").getAttribute("data-state")).toBe("checked");

    fireEvent.click(headerShowDiff());

    expect(screen.getByTestId("diff-stub").textContent).toBe(HTML_PATH);
    expect(
      screen.getByRole("button", { name: "Toggle word wrap" }).getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.queryByTestId("review-html-diff-preview")).toBeNull();
  });

  it("leaves sibling rows and their diffs untouched", async () => {
    mocks.requestFileContent.mockResolvedValue(workspaceContent(WORKSPACE_CONTENT));
    const htmlFile = reviewFile(HTML_PATH, REPO_NAME);
    const sibling = reviewFile("src/app.ts", REPO_NAME);
    renderList([htmlFile, sibling], new Set(), reviewFileKey(sibling));

    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));

    await waitFor(() => expect(screen.getByTestId("review-html-diff-preview")).toBeTruthy());
    expect(screen.getAllByTestId("diff-stub")).toHaveLength(1);
    expect(screen.getByTestId("diff-stub").textContent).toBe("src/app.ts");
  });

  it("reads the complete current workspace file in the selected repository", async () => {
    mocks.requestFileContent.mockResolvedValue(workspaceContent(WORKSPACE_CONTENT));
    renderList([reviewFile(HTML_PATH, REPO_NAME)]);

    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));

    await waitFor(() =>
      expect(publisher.publish).toHaveBeenCalledWith({
        path: HTML_PATH,
        content: WORKSPACE_CONTENT,
        repo: REPO_NAME,
      }),
    );
    expect(mocks.requestFileContent).toHaveBeenCalledWith(
      { id: "ws" },
      SESSION_ID,
      HTML_PATH,
      REPO_NAME,
    );
  });

  it("shows localized recovery with retry, then returns to the same diff", async () => {
    mocks.requestFileContent.mockRejectedValueOnce(new Error("workspace unavailable"));
    mocks.requestFileContent.mockResolvedValueOnce(workspaceContent("<html>Recovered</html>"));
    renderList([reviewFile(HTML_PATH)]);

    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));

    await waitFor(() =>
      expect(screen.getByTestId("html-preview-error").textContent).toContain(
        "Could not read the current HTML file",
      ),
    );
    expect(headerShowDiff()).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Retry HTML preview" }));

    await waitFor(() => expect(screen.getByTestId("html-preview-frame")).toBeTruthy());
    expect(publisher.publish).toHaveBeenCalledWith({
      path: HTML_PATH,
      content: "<html>Recovered</html>",
    });

    fireEvent.click(headerShowDiff());
    expect(screen.getByTestId("diff-stub").textContent).toBe(HTML_PATH);
  });

  it("does not expose the action for deleted, binary, or non-HTML rows", () => {
    renderList([
      { ...reviewFile("site/deleted.html"), status: "deleted" },
      { ...reviewFile("site/binary.html"), diff_skip_reason: "binary" },
      reviewFile("src/app.ts"),
    ]);

    expect(screen.queryByRole("button", { name: PREVIEW_HTML_LABEL })).toBeNull();
    expect(mocks.requestFileContent).not.toHaveBeenCalled();
  });
});
