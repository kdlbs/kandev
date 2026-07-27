import { act, cleanup, render } from "@testing-library/react";
import { EditorState } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  consumePendingCursorPosition,
  scrollEditorIfMounted,
  setPendingCursorPosition,
} from "@/hooks/file-editor-cursor";

const editorHarness = vi.hoisted(() => ({
  onCreateEditor: null as ((view: unknown) => void) | null,
}));
const getMonacoInstance = vi.hoisted(() => vi.fn());

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance,
}));

vi.mock("@uiw/react-codemirror", () => ({
  default: ({ onCreateEditor }: { onCreateEditor: (view: unknown) => void }) => {
    editorHarness.onCreateEditor = onCreateEditor;
    return <div data-testid="codemirror" />;
  },
}));

vi.mock("./use-codemirror-editor-state", () => ({
  useCodeMirrorEditorState: () => ({
    comments: [],
    diffStats: null,
    extensions: [],
    floatingButtonPos: null,
    textSelection: null,
    commentView: null,
    wrapEnabled: false,
    setWrapEnabled: vi.fn(),
    handleChange: vi.fn(),
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

const FILE_PATH = "src/app.ts";
const FRONTEND_REPO = "frontend";
const BACKEND_REPO = "backend";
const CONTENT = "alpha\nbravo\ncharlie";

const baseProps = {
  path: FILE_PATH,
  content: CONTENT,
  originalContent: CONTENT,
  isDirty: false,
  isSaving: false,
  repo: FRONTEND_REPO,
  onChange: vi.fn(),
  onSave: vi.fn(),
};

function createEditorView(content = CONTENT) {
  return {
    state: EditorState.create({ doc: content }),
    dispatch: vi.fn(),
    focus: vi.fn(),
  } as unknown as EditorView;
}

function renderEditor(props = baseProps) {
  return render(
    <TooltipProvider>
      <CodeMirrorCodeEditor {...props} />
    </TooltipProvider>,
  );
}

function mountEditorView(view: EditorView) {
  act(() => editorHarness.onCreateEditor?.(view));
}

afterEach(() => {
  cleanup();
  editorHarness.onCreateEditor = null;
  getMonacoInstance.mockReset();
  consumePendingCursorPosition(FILE_PATH);
  consumePendingCursorPosition(FILE_PATH, FRONTEND_REPO);
  consumePendingCursorPosition(FILE_PATH, BACKEND_REPO);
});

describe("CodeMirrorCodeEditor pending cursor navigation", () => {
  it("reveals the pending 1-based line and column for its repository", () => {
    setPendingCursorPosition(FILE_PATH, 2, 3, FRONTEND_REPO);
    const view = createEditorView();

    renderEditor();
    mountEditorView(view);

    expect(view.dispatch).toHaveBeenCalledWith({
      selection: { anchor: 8 },
      effects: expect.anything(),
    });
    expect(view.focus).toHaveBeenCalledTimes(1);
    expect(consumePendingCursorPosition(FILE_PATH, FRONTEND_REPO)).toBeUndefined();
  });

  it("leaves a pending position for the same path in another repository untouched", () => {
    setPendingCursorPosition(FILE_PATH, 2, 3, FRONTEND_REPO);
    setPendingCursorPosition(FILE_PATH, 3, 2, BACKEND_REPO);
    const view = createEditorView();

    renderEditor();
    mountEditorView(view);

    expect(view.dispatch).toHaveBeenCalledWith({
      selection: { anchor: 8 },
      effects: expect.anything(),
    });
    expect(consumePendingCursorPosition(FILE_PATH, BACKEND_REPO)).toEqual({
      line: 3,
      column: 2,
    });
  });

  it("clamps a pending position to the end of the document", () => {
    setPendingCursorPosition(FILE_PATH, 99, 99, FRONTEND_REPO);
    const view = createEditorView();

    renderEditor();
    mountEditorView(view);

    expect(view.dispatch).toHaveBeenCalledWith({
      selection: { anchor: CONTENT.length },
      effects: expect.anything(),
    });
  });

  it("consumes a pending position when an already-mounted editor updates", () => {
    const view = createEditorView();
    const rendered = renderEditor();
    mountEditorView(view);
    setPendingCursorPosition(FILE_PATH, 2, 3, FRONTEND_REPO);

    rendered.rerender(
      <TooltipProvider>
        <CodeMirrorCodeEditor {...baseProps} isDirty />
      </TooltipProvider>,
    );

    expect(view.dispatch).toHaveBeenCalledWith({
      selection: { anchor: 8 },
      effects: expect.anything(),
    });
    expect(view.focus).toHaveBeenCalledTimes(1);
  });
});

describe("CodeMirrorCodeEditor mounted cursor broker", () => {
  it("synchronously reveals a pending position in an already-mounted pinned editor", () => {
    const view = createEditorView();
    const rendered = renderEditor();
    mountEditorView(view);
    getMonacoInstance.mockReturnValue({
      editor: {
        getEditors: () => [
          {
            getModel: () => ({ uri: { path: "/worktree/src/other.ts" } }),
          },
        ],
      },
    });
    setPendingCursorPosition(FILE_PATH, 2, 3, FRONTEND_REPO);

    const revealed = scrollEditorIfMounted(FILE_PATH, null, 2, 3, FRONTEND_REPO);

    expect(revealed).toBe(true);
    expect(view.dispatch).toHaveBeenCalledWith({
      selection: { anchor: 8 },
      effects: expect.anything(),
    });
    expect(view.focus).toHaveBeenCalledTimes(1);
    expect(consumePendingCursorPosition(FILE_PATH, FRONTEND_REPO)).toBeUndefined();

    rendered.unmount();
    vi.mocked(view.dispatch).mockClear();
    setPendingCursorPosition(FILE_PATH, 3, 1, FRONTEND_REPO);

    expect(scrollEditorIfMounted(FILE_PATH, null, 3, 1, FRONTEND_REPO)).toBe(false);
    expect(view.dispatch).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(FILE_PATH, FRONTEND_REPO)).toEqual({
      line: 3,
      column: 1,
    });
  });

  it("does not reveal the same path from another repository", () => {
    const view = createEditorView();
    renderEditor();
    mountEditorView(view);
    setPendingCursorPosition(FILE_PATH, 3, 2, BACKEND_REPO);

    const revealed = scrollEditorIfMounted(FILE_PATH, null, 3, 2, BACKEND_REPO);

    expect(revealed).toBe(false);
    expect(view.dispatch).not.toHaveBeenCalled();
    expect(view.focus).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(FILE_PATH, BACKEND_REPO)).toEqual({
      line: 3,
      column: 2,
    });
  });
});
