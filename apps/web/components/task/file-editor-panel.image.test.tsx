import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useDockviewStore, type FileEditorState } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";

vi.mock("./file-image-viewer", () => ({
  FileImageViewer: ({ path, content }: { path: string; content: string }) => (
    <div data-testid="image-viewer" data-path={path}>
      {content}
    </div>
  ),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeSessionId: null },
      taskSessions: { items: {} },
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

const makeImageState = (path: string, content: string): FileEditorState => ({
  path,
  name: path.split("/").pop() ?? path,
  content,
  originalContent: content,
  originalHash: `h:${content}`,
  isDirty: false,
  isBinary: true,
});

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
      <FileEditorPanel panelId={PREVIEW_PANEL_ID} params={{ path: "docs/first.png" }} />,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe("first-image");
    await act(async () => {
      await Promise.resolve();
    });

    act(() => seedImage("docs/second.png", "second-image"));
    rerender(<FileEditorPanel panelId={PREVIEW_PANEL_ID} params={{ path: "docs/second.png" }} />);

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe("second-image");
  });

  it("shows different image content for the same path across repos", () => {
    act(() => {
      seedImage(SHARED_IMAGE_PATH, "content-from-repo-a", "repo-a");
      seedImage(SHARED_IMAGE_PATH, "content-from-repo-b", "repo-b");
    });
    const { rerender } = render(
      <FileEditorPanel
        panelId={PREVIEW_PANEL_ID}
        params={{ path: SHARED_IMAGE_PATH, repo: "repo-a" }}
      />,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe("content-from-repo-a");

    rerender(
      <FileEditorPanel
        panelId={PREVIEW_PANEL_ID}
        params={{ path: SHARED_IMAGE_PATH, repo: "repo-b" }}
      />,
    );

    expect(screen.getByTestId(IMAGE_VIEWER_TEST_ID).textContent).toBe("content-from-repo-b");
  });
});
