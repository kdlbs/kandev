import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

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
};
const README_CONTENT = "# README";
const REPORT_CONTENT = "<h1>Report</h1>";
const README_PATH = "README.md";
const REPORT_PATH = "reports/index.html";
const FILE_CONTENT_TEST_ID = "file-content";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  useExternalVcsFileStatus: () => ({ status: "untracked" }),
}));

vi.mock("../file-viewer-content", () => ({
  FileViewerContent: () => <span data-testid={FILE_CONTENT_TEST_ID} />,
}));
vi.mock("../markdown-preview-content", () => ({
  MarkdownPreviewContent: ({ onTogglePreview }: { onTogglePreview: () => void }) => (
    <div data-testid="markdown-preview">
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

afterEach(cleanup);

describe("MobileFileViewerPanel workspace path", () => {
  it("uses the effective workspace path for binary file viewers", () => {
    render(
      <TooltipProvider>
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
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("binary-viewer").getAttribute("data-worktree-path")).toBe(
      "/tmp/task-root",
    );
  });
});

describe("MobileFileViewerPanel external file action", () => {
  it("renders a touch-sized action scoped to the open file's repository", () => {
    render(
      <TooltipProvider>
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
        />
      </TooltipProvider>,
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
});

describe("MobileFileViewerPanel Markdown preview mode", () => {
  it("opens a Markdown file directly in preview mode when requested", () => {
    render(
      <TooltipProvider>
        <MobileFileViewerPanel
          file={{
            path: README_PATH,
            name: README_PATH,
            content: README_CONTENT,
            originalContent: README_CONTENT,
            originalHash: "hash",
            isDirty: false,
          }}
          sessionId="session-1"
          onClose={vi.fn()}
          initialRenderedPreview
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("markdown-preview")).toBeTruthy();
    expect(screen.queryByTestId(FILE_CONTENT_TEST_ID)).toBeNull();
  });

  it("resets preview mode when the same path is opened from another repository", () => {
    const { rerender } = render(
      <TooltipProvider>
        <MobileFileViewerPanel
          file={{
            path: README_PATH,
            name: README_PATH,
            repo: "frontend",
            content: README_CONTENT,
            originalContent: README_CONTENT,
            originalHash: "hash",
            isDirty: false,
          }}
          sessionId="session-1"
          onClose={vi.fn()}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByTestId("markdown-preview-toggle"));
    expect(screen.getByTestId("markdown-preview")).toBeTruthy();

    rerender(
      <TooltipProvider>
        <MobileFileViewerPanel
          file={{
            path: README_PATH,
            name: README_PATH,
            repo: "backend",
            content: README_CONTENT,
            originalContent: README_CONTENT,
            originalHash: "hash",
            isDirty: false,
          }}
          sessionId="session-1"
          onClose={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(FILE_CONTENT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("markdown-preview")).toBeNull();
  });
});

describe("MobileFileViewerPanel HTML preview mode", () => {
  it("previews an HTML file and returns to the source viewer", () => {
    render(
      <TooltipProvider>
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
        />
      </TooltipProvider>,
    );

    const toggle = screen.getByTestId("html-preview-toggle");
    expect(toggle.className).toContain("h-11");
    expect(toggle.className).toContain("w-11");
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
      <TooltipProvider>
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
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(FILE_CONTENT_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId("html-preview-frame")).toBeNull();
  });
});
