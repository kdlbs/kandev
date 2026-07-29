import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const state = {
  taskSessions: {
    items: {
      "session-1": {
        id: "session-1",
        task_id: "task-1",
        repository_id: "primary-repo",
        worktree_path: "/tmp/task",
      },
    },
  },
  tasks: { activeTaskId: "task-1" },
};

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
  FileViewerContent: () => <span data-testid="file-content" />,
}));
vi.mock("../markdown-preview-content", () => ({
  MarkdownPreviewContent: () => <span data-testid="markdown-preview" />,
}));
vi.mock("../file-image-viewer", () => ({ FileImageViewer: () => null }));
vi.mock("../file-binary-viewer", () => ({ FileBinaryViewer: () => null }));

import { MobileFileViewerPanel } from "./mobile-file-viewer-panel";

afterEach(cleanup);

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
          path: "README.md",
          name: "README.md",
          content: "# README",
          originalContent: "# README",
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
        initialMarkdownPreview
      />,
    );

    expect(screen.getByTestId("markdown-preview")).toBeTruthy();
    expect(screen.queryByTestId("file-content")).toBeNull();
  });

  it("resets preview mode when the same path is opened from another repository", () => {
    const { rerender } = render(
      <MobileFileViewerPanel
        file={{
          path: "README.md",
          name: "README.md",
          repo: "frontend",
          content: "# README",
          originalContent: "# README",
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("markdown-preview-toggle"));
    expect(screen.getByTestId("markdown-preview")).toBeTruthy();

    rerender(
      <MobileFileViewerPanel
        file={{
          path: "README.md",
          name: "README.md",
          repo: "backend",
          content: "# README",
          originalContent: "# README",
          originalHash: "hash",
          isDirty: false,
        }}
        sessionId="session-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByTestId("file-content")).toBeTruthy();
    expect(screen.queryByTestId("markdown-preview")).toBeNull();
  });
});
