import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  responsive: { isMobile: true, isFinePointer: true },
  state: {
    value: {
      comments: [],
      diffStats: null,
      extensions: [],
      floatingButtonPos: null as null | { x: number; y: number },
      currentSelection: { text: "first quote", startLine: 2, endLine: 2 },
      textSelection: null,
      commentView: null,
      wrapEnabled: false,
      setWrapEnabled: vi.fn(),
      handleChange: vi.fn(),
      handleFloatingButtonClick: vi.fn(),
      handleDockButtonClick: vi.fn(),
    },
  },
}));

vi.mock("@uiw/react-codemirror", () => ({ default: () => <div data-testid="codemirror" /> }));
vi.mock("./use-codemirror-editor-state", () => ({
  useCodeMirrorEditorState: () => mocks.state.value,
}));
vi.mock("./use-codemirror-walkthrough-range", () => ({
  useCodeMirrorWalkthroughRange: () => null,
}));
vi.mock("./codemirror-editor-toolbar", () => ({ CodeMirrorToolbar: () => null }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ ...mocks.responsive }),
}));
vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

import { CodeMirrorCodeEditor } from "./codemirror-code-editor";

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
  if (!owner) throw new Error("CodeMirror editor owner was not rendered");
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
    textSelection: null,
    commentView: null,
  };
  vi.restoreAllMocks();
});

describe("CodeMirror touch comment dock", () => {
  it("keeps one owner-anchored 44px action as the selected quote changes", async () => {
    const view = render(<CodeMirrorCodeEditor {...props} enableComments />);
    const owner = editorOwner(view.container);
    fireEvent(window, new Event("resize"));

    const dock = await screen.findByTestId("selection-comment-dock");
    const action = screen.getByTestId("codemirror-comment-button");
    const firstStyle = dock.getAttribute("style");
    expect(action.getAttribute("aria-label")).toBe("Comment on selection");
    expect(action.style.minHeight).toBe("44px");
    expect(owner.style.paddingBottom).toContain("56px");

    mocks.state.value = {
      ...mocks.state.value,
      currentSelection: { text: "updated selected quote", startLine: 8, endLine: 9 },
    };
    view.rerender(<CodeMirrorCodeEditor {...props} enableComments />);
    fireEvent(window, new Event("resize"));
    await waitFor(() => expect(dock.getAttribute("style")).toBe(firstStyle));

    fireEvent.pointerDown(action, { pointerType: "touch" });
    fireEvent.click(action);
    expect(mocks.state.value.handleDockButtonClick).toHaveBeenCalled();
    expect(screen.getAllByTestId("selection-comment-dock")).toHaveLength(1);
  });

  it("leaves the existing selection trigger to fine-pointer desktop rendering", () => {
    mocks.responsive.isMobile = false;
    mocks.state.value = {
      ...mocks.state.value,
      currentSelection: { text: "desktop selection", startLine: 1, endLine: 1 },
      floatingButtonPos: { x: 120, y: 80 },
    };
    render(<CodeMirrorCodeEditor {...props} enableComments />);

    expect(screen.getByRole("button", { name: "Comment" })).toBeTruthy();
    expect(screen.queryByTestId("selection-comment-dock")).toBeNull();
  });
});
