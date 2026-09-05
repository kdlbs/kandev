import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileEditorContentProps } from "./file-editor-content";
import { FileEditorContent } from "./file-editor-content";

vi.mock("@/hooks/use-editor-resolver", () => ({
  useEditorProvider: () => "monaco",
}));

vi.mock("@/components/editors/monaco/monaco-code-editor", () => ({
  MonacoCodeEditor: (props: { path: string; content: string; onPreviewHtml?: () => void }) => (
    <div data-testid="monaco-editor" data-path={props.path} data-content={props.content}>
      {props.onPreviewHtml && (
        <button type="button" onClick={props.onPreviewHtml}>
          Preview HTML
        </button>
      )}
    </div>
  ),
}));

vi.mock("@/components/editors/codemirror/codemirror-code-editor", () => ({
  CodeMirrorCodeEditor: () => <div data-testid="codemirror-editor" />,
}));

vi.mock("./markdown-preview-content", () => ({
  MarkdownPreviewContent: (props: { content: string }) => (
    <div data-testid="markdown-preview" data-content={props.content} />
  ),
}));

afterEach(cleanup);

const baseProps: FileEditorContentProps = {
  path: "reports/index.html",
  content: "<h1>Report</h1>",
  originalContent: "<h1>Old report</h1>",
  isDirty: true,
  isSaving: false,
  onChange: vi.fn(),
  onSave: vi.fn(),
};

describe("FileEditorContent preview selection", () => {
  it("keeps HTML in the source editor and forwards its preview action", () => {
    const onPreviewHtml = vi.fn();
    render(
      <FileEditorContent
        {...baseProps}
        previewKind="html"
        renderedPreview
        onPreviewHtml={onPreviewHtml}
      />,
    );

    expect(screen.getByTestId("monaco-editor").getAttribute("data-content")).toBe(
      "<h1>Report</h1>",
    );
    screen.getByRole("button", { name: "Preview HTML" }).click();
    expect(onPreviewHtml).toHaveBeenCalledOnce();
  });

  it("keeps Markdown preview selection separate from HTML preview", () => {
    render(
      <FileEditorContent
        {...baseProps}
        path="README.md"
        previewKind="markdown"
        renderedPreview
        onTogglePreview={vi.fn()}
      />,
    );

    expect(screen.getByTestId("markdown-preview").getAttribute("data-content")).toBe(
      "<h1>Report</h1>",
    );
    expect(screen.queryByTestId("html-preview")).toBeNull();
  });

  it("returns to the configured source editor when preview is inactive", () => {
    render(
      <FileEditorContent
        {...baseProps}
        previewKind="html"
        renderedPreview={false}
        onTogglePreview={vi.fn()}
      />,
    );

    expect(screen.getByTestId("monaco-editor")).toBeTruthy();
    expect(screen.queryByTestId("html-preview")).toBeNull();
  });
});
