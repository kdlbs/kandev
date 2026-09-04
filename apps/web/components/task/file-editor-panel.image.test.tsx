import { act, cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";

vi.mock("./file-image-viewer", () => ({
  FileImageViewer: ({
    path,
    content,
    worktreePath,
    headerActions,
  }: {
    path: string;
    content: string;
    worktreePath?: string;
    headerActions?: React.ReactNode;
  }) => (
    <div data-testid="image-viewer" data-path={path} data-worktree-path={worktreePath}>
      {content}
      {headerActions}
    </div>
  ),
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  useExternalVcsFileStatus: () => ({ status: "modified" }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeSessionId: "session-1", activeTaskId: "task-1" },
      taskSessions: {
        items: {
          "session-1": {
            workspace_path: "/tmp/task-root",
            worktree_path: "/tmp/task-root/kandev",
            repository_id: "repo-1",
          },
        },
      },
    }),
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({
    savingFiles: new Set<string>(),
    handleFileChange: vi.fn(),
    saveFile: vi.fn(),
    deleteFile: vi.fn(),
    openFile: vi.fn(),
    openFileInMarkdownPreview: vi.fn(),
    applyRemoteUpdate: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
}));

import { FileEditorPanel } from "./file-editor-panel";

const PREVIEW_PANEL_ID = "preview:file-editor";
const IMAGE_VIEWER_TEST_ID = "image-viewer";
const SHARED_IMAGE_PATH = "docs/shared.png";
const REPO_A_CONTENT = "content-from-repo-a";
const REPO_B_CONTENT = "content-from-repo-b";

const makeImageState = (path: string, content: string): FileEditorState => ({
  path,
  name: path.split("/").pop() ?? path,
  content,
  originalContent: content,
  originalHash: `h:${content}`,
  isDirty: false,
  isBinary: true,
});

afterEach(cleanup);

function seedImage(path: string, content: string, repo?: string) {
  useDockviewStore.getState().setFileState(buildRepoScopedItemId(path, repo), {
    ...makeImageState(path, content),
    repo,
  });
}

describe("FileEditorPanel image preview", () => {
  beforeEach(() => {
    act(() => {
      useDockviewStore.getState().clearFileStates();
    });
  });

  it("updates displayed image content when a reused preview tab switches files", async () => {
    act(() => seedImage("docs/first.png", "first-image"));
    const { rerender } = render(
      <TooltipProvider>
        <FileEditorPanel panelId={PREVIEW_PANEL_ID} params={{ path: "docs/first.png" }} />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe("first-image");
    await act(async () => {
      await Promise.resolve();
    });

    act(() => seedImage("docs/second.png", "second-image"));
    rerender(
      <TooltipProvider>
        <FileEditorPanel panelId={PREVIEW_PANEL_ID} params={{ path: "docs/second.png" }} />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe("second-image");
  });

  it("shows different image content for the same path across repos", () => {
    act(() => {
      seedImage(SHARED_IMAGE_PATH, REPO_A_CONTENT, "repo-a");
      seedImage(SHARED_IMAGE_PATH, REPO_B_CONTENT, "repo-b");
    });
    const { rerender } = render(
      <TooltipProvider>
        <FileEditorPanel
          panelId={PREVIEW_PANEL_ID}
          params={{ path: SHARED_IMAGE_PATH, repo: "repo-a" }}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe(REPO_A_CONTENT);

    rerender(
      <TooltipProvider>
        <FileEditorPanel
          panelId={PREVIEW_PANEL_ID}
          params={{ path: SHARED_IMAGE_PATH, repo: "repo-b" }}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe(REPO_B_CONTENT);
  });

  it("shows the shared external action in the docked desktop image header", () => {
    act(() => seedImage(SHARED_IMAGE_PATH, REPO_A_CONTENT, "repo-a"));

    render(
      <TooltipProvider>
        <FileEditorPanel
          panelId={PREVIEW_PANEL_ID}
          params={{ path: SHARED_IMAGE_PATH, repo: "repo-a" }}
        />
      </TooltipProvider>,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: SHARED_IMAGE_PATH,
      status: "modified",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryName: "repo-a",
      size: "sm",
    });
  });

  it("uses the effective workspace path for docked desktop viewers", () => {
    act(() => seedImage(SHARED_IMAGE_PATH, REPO_A_CONTENT, "repo-a"));

    render(
      <TooltipProvider>
        <FileEditorPanel
          panelId={PREVIEW_PANEL_ID}
          params={{ path: SHARED_IMAGE_PATH, repo: "repo-a" }}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).getAttribute("data-worktree-path")).toBe(
      "/tmp/task-root",
    );
  });
});
