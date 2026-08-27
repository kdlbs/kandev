import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { MarkdownFileEditor } from "./markdown-file-editor";

vi.mock("./file-editor-content", () => ({
  FileEditorContent: ({
    markdownPreview,
    onChange,
    onSave,
  }: {
    markdownPreview?: boolean;
    onChange: (value: string) => void;
    onSave: () => void;
  }) => (
    <div
      data-testid="source-editor"
      {...(markdownPreview === undefined
        ? {}
        : { "data-markdown-preview": String(markdownPreview) })}
    >
      <button type="button" onClick={() => onChange("# edited")}>
        Change
      </button>
      <button type="button" onClick={onSave}>
        Save source
      </button>
    </div>
  ),
}));

vi.mock("./markdown-preview-content", () => ({
  MarkdownPreviewContent: ({ content }: { content: string }) => (
    <div data-testid="markdown-preview-content">{content}</div>
  ),
}));

vi.mock("@/components/editors/markdown/hybrid-markdown-editor", () => ({
  HybridMarkdownEditor: ({
    content,
    onChange,
  }: {
    content: string;
    onChange: (value: string) => void;
  }) => (
    <div data-testid="hybrid-editor" data-content={content}>
      <button type="button" onClick={() => onChange("# hybrid edit")}>
        Change hybrid
      </button>
    </div>
  ),
}));

vi.mock("@/components/task/file-viewer-header", () => ({
  FileViewerExternalLink: () => <span data-testid="external-link" />,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

afterEach(cleanup);

const baseProps = {
  path: "README.md",
  content: "# source",
  originalContent: "# source",
  isDirty: false,
  isSaving: false,
  sessionId: "session-1",
  taskId: "task-1",
  repositoryId: "repo-1",
  worktreePath: "/tmp/worktree",
  repo: "frontend",
  onModeChange: vi.fn(),
  onChange: vi.fn(),
  onSave: vi.fn(),
  onReloadFromAgent: vi.fn(),
  onDelete: vi.fn(),
};

describe("MarkdownFileEditor", () => {
  it("renders Preview and changes mode through one visible mode control", () => {
    render(<MarkdownFileEditor {...baseProps} mode="preview" />);

    expect(screen.getByTestId("markdown-preview-content")).toBeTruthy();
    expect(screen.queryByTestId("hybrid-editor")).toBeNull();
    fireEvent.click(screen.getByTestId("markdown-mode-edit"));
    expect(baseProps.onModeChange).toHaveBeenCalledWith("edit");
  });

  it("renders the hybrid editor in Edit mode and forwards source changes", () => {
    const onChange = vi.fn();
    render(<MarkdownFileEditor {...baseProps} mode="edit" onChange={onChange} />);

    expect(screen.getByTestId("hybrid-editor").getAttribute("data-content")).toBe("# source");
    fireEvent.click(screen.getByRole("button", { name: "Change hybrid" }));
    expect(onChange).toHaveBeenCalledWith("# hybrid edit");
    expect((screen.getByTestId("markdown-file-save") as HTMLButtonElement).disabled).toBe(true);
  });

  it("uses the existing source editor and keeps Save available in Source mode", () => {
    render(<MarkdownFileEditor {...baseProps} mode="source" isDirty />);

    expect(screen.getByTestId("source-editor").getAttribute("data-markdown-preview")).toBeNull();
    expect(screen.getByTestId("markdown-mode-source").getAttribute("aria-pressed")).toBe("true");
    expect((screen.getByTestId("markdown-file-save") as HTMLButtonElement).disabled).toBe(false);
  });

  it("omits Edit for MDX while keeping Preview and Source", () => {
    render(<MarkdownFileEditor {...baseProps} path="README.mdx" mode="preview" />);

    expect(screen.queryByTestId("markdown-mode-edit")).toBeNull();
    expect(screen.getByTestId("markdown-mode-preview")).toBeTruthy();
    expect(screen.getByTestId("markdown-mode-source")).toBeTruthy();
  });
});
