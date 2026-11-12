import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  responsive: { isMobile: true, isFinePointer: true },
  state: {
    value: {
      wrapEnabled: false,
      setWrapEnabled: vi.fn(),
      showDiffIndicators: false,
      setShowDiffIndicators: vi.fn(),
      editorInstance: null,
      editorRef: { current: null },
      decorationsRef: { current: null },
      diffDecorationsRef: { current: null },
      formZoneRange: null as null | { startLine: number; endLine: number; codeContent?: string },
      setFormZoneRange: vi.fn(),
      floatingButtonPos: null as null | { x: number; y: number },
      currentSelection: { text: "first quote", startLine: 2, endLine: 2 },
      editingCommentId: null,
      setEditingComment: vi.fn(),
      comments: [],
      clearGutterSelection: vi.fn(),
      handleEditorDidMount: vi.fn(),
      handleChange: vi.fn(),
      handleFloatingButtonClick: vi.fn(),
      handleDockButtonClick: vi.fn(),
      handleCommentSubmitRef: { current: vi.fn() },
      handleCommentSubmitAndRunRef: { current: vi.fn() },
      handleCommentDeleteRef: { current: vi.fn() },
      handleCommentUpdateRef: { current: vi.fn() },
      handleCommentRunRef: { current: vi.fn() },
    },
  },
  lsp: {
    monacoPath: "file:///workspace/src/example.ts",
    lspStatus: { state: "disabled" },
    lspProgress: {
      initializingSince: null,
      active: [],
      completed: null,
      hasReportedProgress: false,
    },
    lspLanguage: null,
    toggleLsp: vi.fn(),
  },
}));

vi.mock("@monaco-editor/react", () => ({
  default: (props: { options?: { padding?: { bottom?: number } } }) => (
    <div data-testid="monaco-editor" data-padding-bottom={props.options?.padding?.bottom ?? 0} />
  ),
}));
vi.mock("./use-monaco-editor-state", () => ({
  useMonacoEditorComments: () => mocks.state.value,
}));
vi.mock("./monaco-editor-toolbar", () => ({ MonacoEditorToolbar: () => null }));
vi.mock("./use-monaco-editor-lsp", () => ({
  useMonacoEditorLsp: () => mocks.lsp,
  useMonacoDiffDecorations: () => ({ diffStats: null }),
}));
vi.mock("./use-monaco-walkthrough-range", () => ({ useMonacoWalkthroughRange: () => null }));
vi.mock("./use-monaco-cursor-navigation", () => ({ useMonacoCursorNavigation: vi.fn() }));
vi.mock("@/hooks/use-editor-view-zone-comments", () => ({ useEditorViewZoneComments: vi.fn() }));
vi.mock("@/components/theme/app-theme", () => ({ useTheme: () => ({ resolvedTheme: "dark" }) }));
vi.mock("@/hooks/use-lsp-status-placement", () => ({ useLspStatusPlacement: () => "toolbar" }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ ...mocks.responsive }),
}));
vi.mock("./monaco-init", () => ({ initMonacoThemes: vi.fn() }));

import { MonacoCodeEditor } from "./monaco-code-editor";

const props = {
  path: "src/example.ts",
  content: "line one\nline two\nline three",
  originalContent: "line one\nline two\nline three",
  isDirty: false,
  isSaving: false,
  sessionId: "session-1",
  onChange: vi.fn(),
  onSave: vi.fn(),
};

function editorOwner(container: HTMLElement) {
  const owner = container.querySelector<HTMLElement>(".flex-1.overflow-hidden.relative");
  if (!owner) throw new Error("Monaco editor owner was not rendered");
  owner.getBoundingClientRect = () => ({
    left: 32,
    top: 80,
    right: 432,
    bottom: 680,
    width: 400,
    height: 600,
    x: 32,
    y: 80,
    toJSON: () => ({}),
  });
  return owner;
}

afterEach(() => {
  cleanup();
  mocks.responsive.isMobile = true;
  mocks.responsive.isFinePointer = true;
  mocks.state.value = {
    ...mocks.state.value,
    currentSelection: { text: "first quote", startLine: 2, endLine: 2 },
    floatingButtonPos: null,
    formZoneRange: null,
  };
  vi.restoreAllMocks();
});

describe("Monaco touch comment dock", () => {
  it("keeps one owner-anchored 44px action as the selected quote changes", async () => {
    const view = render(<MonacoCodeEditor {...props} enableComments />);
    editorOwner(view.container);
    fireEvent(window, new Event("resize"));

    const dock = await screen.findByTestId("selection-comment-dock");
    const action = screen.getByTestId("monaco-comment-button");
    const firstStyle = dock.getAttribute("style");
    expect(action.getAttribute("aria-label")).toBe("Comment on selection");
    expect(action.style.minHeight).toBe("44px");
    expect(screen.getByTestId("monaco-editor").getAttribute("data-padding-bottom")).toBe("56");

    mocks.state.value = {
      ...mocks.state.value,
      currentSelection: { text: "updated selected quote", startLine: 8, endLine: 9 },
    };
    view.rerender(<MonacoCodeEditor {...props} enableComments />);
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(dock.getAttribute("style")).toBe(firstStyle));

    fireEvent.pointerDown(action, { pointerType: "touch" });
    fireEvent.click(action);
    expect(mocks.state.value.handleDockButtonClick).toHaveBeenCalled();

    mocks.state.value = { ...mocks.state.value, formZoneRange: { startLine: 8, endLine: 9 } };
    view.rerender(<MonacoCodeEditor {...props} enableComments />);
    await waitFor(() => expect(screen.queryByTestId("selection-comment-dock")).toBeNull());
    expect(screen.getByTestId("monaco-editor").getAttribute("data-padding-bottom")).toBe("120");
  });

  it("leaves the existing selection trigger to fine-pointer desktop rendering", () => {
    mocks.responsive.isMobile = false;
    mocks.state.value = {
      ...mocks.state.value,
      currentSelection: { text: "desktop selection", startLine: 1, endLine: 1 },
      floatingButtonPos: { x: 120, y: 80 },
    };
    render(<MonacoCodeEditor {...props} enableComments />);

    expect(screen.getByRole("button", { name: "Comment" })).toBeTruthy();
    expect(screen.queryByTestId("selection-comment-dock")).toBeNull();
  });
});
