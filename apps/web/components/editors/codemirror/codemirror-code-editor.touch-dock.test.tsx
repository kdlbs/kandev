import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

const STATE = vi.hoisted(() => ({ touch: false, open: vi.fn() }));

vi.mock("@uiw/react-codemirror", () => ({
  default: () => <div data-testid="codemirror" />,
}));

vi.mock("./use-codemirror-editor-state", () => ({
  useCodeMirrorEditorState: () => ({
    touch: STATE.touch,
    comments: [],
    diffStats: null,
    extensions: [],
    currentSelection: { text: "const a = 1;", startLine: 1, endLine: 1 },
    floatingButtonPos: { x: 40, y: 60 },
    textSelection: null,
    commentView: null,
    wrapEnabled: false,
    setWrapEnabled: vi.fn(),
    handleChange: vi.fn(),
    handleFloatingButtonClick: STATE.open,
  }),
}));

vi.mock("./use-codemirror-walkthrough-range", () => ({
  useCodeMirrorWalkthroughRange: () => null,
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

import { CodeMirrorCodeEditor } from "./codemirror-code-editor";

afterEach(() => {
  cleanup();
  STATE.touch = false;
  STATE.open.mockClear();
});

function renderEditor() {
  render(
    <TooltipProvider>
      <CodeMirrorCodeEditor
        path="src/index.ts"
        content="const a = 1;"
        originalContent=""
        isDirty={false}
        isSaving={false}
        sessionId="session-1"
        enableComments
        onChange={vi.fn()}
        onSave={vi.fn()}
      />
    </TooltipProvider>,
  );
}

describe("CodeMirrorCodeEditor selection comment trigger", () => {
  it("floats the comment button beside the selection with a fine pointer", () => {
    renderEditor();

    const button = screen.getByTestId("code-editor-comment-button");
    expect(button.classList.contains("floating-comment-btn")).toBe(true);
    expect(screen.queryByRole("toolbar", { name: "Selection actions" })).toBeNull();
  });

  it("docks the comment button in a toolbar on touch", () => {
    STATE.touch = true;
    renderEditor();

    const toolbar = screen.getByRole("toolbar", { name: "Selection actions" });
    const button = screen.getByTestId("code-editor-comment-button");
    expect(button.parentElement).toBe(toolbar);
    fireEvent.click(button);
    expect(STATE.open).toHaveBeenCalledTimes(1);
  });
});
