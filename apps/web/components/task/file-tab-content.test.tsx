import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { OpenFileTab } from "@/lib/types/backend";
import { FileTabContent } from "./file-tab-content";

vi.mock("./file-editor-content", () => ({
  FileEditorContent: ({
    markdownMode,
    worktreePath,
    onToggleMarkdownPreview,
  }: {
    markdownMode?: "preview" | "edit" | "source";
    worktreePath?: string;
    onToggleMarkdownPreview?: () => void;
  }) => (
    <div
      data-testid="file-editor-content"
      data-markdown-mode={markdownMode}
      data-worktree-path={worktreePath}
    >
      <button type="button" onClick={onToggleMarkdownPreview}>
        Toggle preview
      </button>
    </div>
  ),
}));

vi.mock("./markdown-file-editor", () => ({
  MarkdownFileEditor: ({
    mode,
    onModeChange,
    onSourceFallback,
    onOpenLink,
  }: {
    mode: "preview" | "edit" | "source";
    onModeChange: (mode: "preview" | "edit" | "source") => void;
    onSourceFallback?: () => void;
    onOpenLink?: (url: string) => void;
  }) => (
    <div data-testid="markdown-file-editor" data-markdown-mode={mode}>
      <button type="button" onClick={() => onModeChange("source")}>
        Toggle mode
      </button>
      <button type="button" onClick={onSourceFallback}>
        Fallback to source
      </button>
      <button type="button" onClick={() => onOpenLink?.("./guide.md")}>
        Open Markdown link
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
  markdownMode: "preview",
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
        onMarkdownModeChange={onToggleMarkdownPreview}
      />,
    );

    expect(screen.getByTestId("markdown-file-editor").getAttribute("data-markdown-mode")).toBe(
      "preview",
    );
    fireEvent.click(screen.getByRole("button", { name: "Toggle mode" }));
    expect(onToggleMarkdownPreview).toHaveBeenCalledWith("source");
  });

  it("wires the Task Center Markdown fallback back to the mode owner", () => {
    const onMarkdownModeChange = vi.fn();

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
        onMarkdownModeChange={onMarkdownModeChange}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Fallback to source" }));

    expect(onMarkdownModeChange).toHaveBeenCalledWith("source");
  });

  it("routes Edit-mode Markdown links through the Task Center file opener", () => {
    const onOpenFile = vi.fn();

    render(
      <FileTabContent
        tab={{ ...file, path: "docs/readme.md", name: "readme.md" }}
        activeSession={{ workspace_path: "/tmp/task-root" }}
        activeSessionId="session-1"
        taskId="task-1"
        isSaving={false}
        onFileChange={vi.fn()}
        onFileSave={vi.fn()}
        onFileDelete={vi.fn()}
        onOpenFile={onOpenFile}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Open Markdown link" }));

    expect(onOpenFile).toHaveBeenCalledWith("docs/guide.md", undefined);
  });

  it("uses the effective workspace path for desktop file viewers", () => {
    render(
      <FileTabContent
        tab={{ ...file, path: "README.txt", name: "README.txt" }}
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
