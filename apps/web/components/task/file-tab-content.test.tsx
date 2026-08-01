import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { OpenFileTab } from "@/lib/types/backend";
import { FileTabContent } from "./file-tab-content";

vi.mock("./file-editor-content", () => ({
  FileEditorContent: ({
    markdownPreview,
    worktreePath,
    onToggleMarkdownPreview,
  }: {
    markdownPreview?: boolean;
    worktreePath?: string;
    onToggleMarkdownPreview?: () => void;
  }) => (
    <div
      data-testid="file-editor-content"
      data-markdown-preview={String(markdownPreview)}
      data-worktree-path={worktreePath}
    >
      <button type="button" onClick={onToggleMarkdownPreview}>
        Toggle preview
      </button>
    </div>
  ),
}));

vi.mock("@kandev/ui/tabs", () => ({
  TabsContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("./file-viewer-header", () => ({
  FileViewerExternalLink: () => null,
}));

vi.mock("./file-image-viewer", () => ({ FileImageViewer: () => null }));
vi.mock("./file-binary-viewer", () => ({ FileBinaryViewer: () => null }));

const file: OpenFileTab = {
  path: "README.md",
  name: "README.md",
  content: "# README",
  originalContent: "# README",
  originalHash: "hash",
  isDirty: false,
  markdownPreview: true,
};

afterEach(cleanup);

describe("FileTabContent Markdown preview", () => {
  it("renders a Markdown tab in preview mode and forwards the toggle", () => {
    const onToggleMarkdownPreview = vi.fn();

    render(
      <FileTabContent
        tab={file}
        activeSession={null}
        activeSessionId="session-1"
        taskId="task-1"
        isSaving={false}
        onFileChange={vi.fn()}
        onFileSave={vi.fn()}
        onFileDelete={vi.fn()}
        onToggleMarkdownPreview={onToggleMarkdownPreview}
      />,
    );

    expect(screen.getByTestId("file-editor-content").getAttribute("data-markdown-preview")).toBe(
      "true",
    );
    fireEvent.click(screen.getByRole("button", { name: "Toggle preview" }));
    expect(onToggleMarkdownPreview).toHaveBeenCalledOnce();
  });

  it("uses the effective workspace path for desktop file viewers", () => {
    render(
      <FileTabContent
        tab={file}
        activeSession={{
          workspace_path: "/tmp/task-root",
          worktree_path: "/tmp/task-root/kandev",
        }}
        activeSessionId="session-1"
        taskId="task-1"
        isSaving={false}
        onFileChange={vi.fn()}
        onFileSave={vi.fn()}
        onFileDelete={vi.fn()}
      />,
    );

    expect(screen.getByTestId("file-editor-content").getAttribute("data-worktree-path")).toBe(
      "/tmp/task-root",
    );
  });
});
