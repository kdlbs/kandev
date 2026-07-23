import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@kandev/ui/tabs", () => ({
  TabsContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("./file-image-viewer", () => ({
  FileImageViewer: ({ headerActions }: { headerActions?: React.ReactNode }) => (
    <div data-testid="image-viewer">{headerActions}</div>
  ),
}));

vi.mock("./file-binary-viewer", () => ({
  FileBinaryViewer: ({ headerActions }: { headerActions?: React.ReactNode }) => (
    <div data-testid="binary-viewer">{headerActions}</div>
  ),
}));

vi.mock("./file-editor-content", () => ({ FileEditorContent: () => null }));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  useExternalVcsFileStatus: () => ({ status: "modified" }),
}));

import { FileTabContent } from "./file-tab-content";

afterEach(cleanup);

const activeSession = { worktree_path: "/tmp/worktree", repository_id: "repo-1" };

describe("FileTabContent external file action", () => {
  it.each([
    ["image", "assets/logo.png", "image/png", "image-viewer"],
    ["binary", "dist/archive.zip", "binary", "binary-viewer"],
  ])("shows the shared action for a desktop %s viewer", (_kind, path, content, viewerId) => {
    render(
      <FileTabContent
        tab={{
          path,
          name: path.split("/").pop() ?? path,
          repo: "frontend",
          content,
          originalContent: content,
          originalHash: "hash",
          isDirty: false,
          isBinary: true,
        }}
        activeSession={activeSession}
        activeSessionId="session-1"
        taskId="task-1"
        isSaving={false}
        onFileChange={vi.fn()}
        onFileSave={vi.fn()}
        onFileDelete={vi.fn()}
      />,
    );

    expect(screen.getByTestId(viewerId)).toBeTruthy();
    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: path,
      status: "modified",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryName: "frontend",
      size: "sm",
    });
  });
});
