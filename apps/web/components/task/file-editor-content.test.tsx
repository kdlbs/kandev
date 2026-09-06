import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FileEditorContentProps } from "./file-editor-content";
import { FileEditorContent } from "./file-editor-content";

const htmlPreview = vi.hoisted(() => ({
  status: "ready" as const,
  url: "http://api.test/port-proxy/session-1/43127/reports/index.html?v=4",
  error: null,
  isPublishing: false,
  publish: vi
    .fn()
    .mockResolvedValue("http://api.test/port-proxy/session-1/43127/reports/index.html?v=4"),
  reset: vi.fn(),
}));

const openBrowserPanel = vi.hoisted(() => vi.fn());
const HTML_CONTENT = "<h1>Report</h1>";
const HTML_PREVIEW_TEST_ID = "html-preview";
const HTML_PATH = "reports/index.html";
const MONACO_EDITOR_TEST_ID = "monaco-editor";
const PREVIEW_HTML_LABEL = "Preview HTML";
const SESSION_ID = "session-1";

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

vi.mock("@/hooks/use-html-preview-publisher", () => ({
  useHtmlPreviewPublisher: () => htmlPreview,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { openBrowserPanel: typeof openBrowserPanel }) => unknown) =>
    selector({ openBrowserPanel }),
}));

vi.mock("./html-preview-content", () => ({
  HtmlPreviewContent: ({
    previewUrl,
    onTogglePreview,
    onRefresh,
    onOpenInBrowser,
  }: {
    previewUrl?: string | null;
    onTogglePreview: () => void;
    onRefresh?: () => void;
    onOpenInBrowser?: () => void;
  }) => (
    <div data-testid="html-preview" data-url={previewUrl}>
      <button type="button" onClick={onTogglePreview}>
        Show code
      </button>
      <button type="button" onClick={onRefresh}>
        Refresh HTML preview
      </button>
      <button type="button" onClick={onOpenInBrowser}>
        Open in Browser panel
      </button>
    </div>
  ),
}));

vi.mock("./markdown-preview-content", () => ({
  MarkdownPreviewContent: (props: { content: string }) => (
    <div data-testid="markdown-preview" data-content={props.content} />
  ),
}));

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
});

const baseProps: FileEditorContentProps = {
  path: HTML_PATH,
  content: HTML_CONTENT,
  originalContent: "<h1>Old report</h1>",
  isDirty: true,
  isSaving: false,
  onChange: vi.fn(),
  onSave: vi.fn(),
};

describe("FileEditorContent preview selection", () => {
  it("replaces HTML source with the published iframe and restores the unchanged buffer", () => {
    render(<FileEditorContent {...baseProps} sessionId={SESSION_ID} previewKind="html" />);

    expect(screen.getByTestId(MONACO_EDITOR_TEST_ID).getAttribute("data-content")).toBe(
      HTML_CONTENT,
    );
    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));

    expect(htmlPreview.publish).toHaveBeenCalledWith({
      path: HTML_PATH,
      content: HTML_CONTENT,
    });
    expect(screen.queryByTestId(MONACO_EDITOR_TEST_ID)).toBeNull();
    expect(screen.getByTestId(HTML_PREVIEW_TEST_ID).getAttribute("data-url")).toBe(htmlPreview.url);

    fireEvent.click(screen.getByRole("button", { name: "Show code" }));
    expect(screen.getByTestId(MONACO_EDITOR_TEST_ID).getAttribute("data-content")).toBe(
      HTML_CONTENT,
    );
    expect(baseProps.onChange).not.toHaveBeenCalled();
  });

  it("opens a Browser panel only from the explicit preview action", () => {
    render(<FileEditorContent {...baseProps} sessionId={SESSION_ID} previewKind="html" />);

    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));
    expect(openBrowserPanel).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Open in Browser panel" }));
    expect(openBrowserPanel).toHaveBeenCalledWith(htmlPreview.url);
  });

  it("refreshes from the latest unsaved buffer", () => {
    const { rerender } = render(
      <FileEditorContent {...baseProps} sessionId={SESSION_ID} previewKind="html" />,
    );
    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));

    rerender(
      <FileEditorContent
        {...baseProps}
        content="<h1>Updated report</h1>"
        sessionId={SESSION_ID}
        previewKind="html"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Refresh HTML preview" }));

    expect(htmlPreview.publish).toHaveBeenLastCalledWith({
      path: HTML_PATH,
      content: "<h1>Updated report</h1>",
    });
  });

  it("restores source and invalidates the published URL when file identity changes", () => {
    const { rerender } = render(
      <FileEditorContent {...baseProps} sessionId={SESSION_ID} previewKind="html" />,
    );
    fireEvent.click(screen.getByRole("button", { name: PREVIEW_HTML_LABEL }));
    expect(screen.getByTestId(HTML_PREVIEW_TEST_ID)).toBeTruthy();

    rerender(
      <FileEditorContent
        {...baseProps}
        path="reports/next.html"
        sessionId="session-2"
        previewKind="html"
      />,
    );

    expect(screen.getByTestId(MONACO_EDITOR_TEST_ID).getAttribute("data-path")).toBe(
      "reports/next.html",
    );
    expect(screen.queryByTestId(HTML_PREVIEW_TEST_ID)).toBeNull();
    expect(htmlPreview.reset).toHaveBeenCalled();

    rerender(<FileEditorContent {...baseProps} sessionId={SESSION_ID} previewKind="html" />);
    expect(screen.getByTestId(MONACO_EDITOR_TEST_ID).getAttribute("data-path")).toBe(HTML_PATH);
    expect(screen.queryByTestId(HTML_PREVIEW_TEST_ID)).toBeNull();
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
    expect(screen.queryByTestId(HTML_PREVIEW_TEST_ID)).toBeNull();
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

    expect(screen.getByTestId(MONACO_EDITOR_TEST_ID)).toBeTruthy();
    expect(screen.queryByTestId(HTML_PREVIEW_TEST_ID)).toBeNull();
  });
});
