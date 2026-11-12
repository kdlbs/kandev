import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { EditorState } from "@codemirror/state";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { useMonacoEditorComments } from "./monaco/use-monaco-editor-state";
import { useCodeMirrorEditorState } from "./codemirror/use-codemirror-editor-state";

const LATEST_QUOTE = "latest quote";

vi.mock("@/components/state-provider", () => ({ useAppStore: () => "task" }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/lib/commands/command-registry", () => ({
  useCommandPanelOpen: () => ({ setOpen: vi.fn() }),
}));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/hooks/use-gutter-comments", () => ({
  useGutterComments: () => ({ clearGutterSelection: vi.fn() }),
}));

beforeEach(() => {
  sessionStorage.clear();
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it.each(["api", ""])("Monaco creates and selects comments in scope %j", (repo) => {
  const props = {
    path: "README.md",
    sessionId: "s",
    enableComments: true,
    wrapperRef: { current: null },
    contentRef: { current: "text" },
    onChange: vi.fn(),
    onSave: vi.fn(),
  };
  const { result } = renderHook(() => ({
    selected: useMonacoEditorComments({ ...props, repo }),
    other: useMonacoEditorComments({ ...props, repo: "other" }),
  }));
  act(() =>
    result.current.selected.setFormZoneRange({ startLine: 1, endLine: 1, codeContent: "text" }),
  );
  act(() => result.current.selected.handleCommentSubmitRef.current("Feedback"));
  expect(result.current.selected.comments[0]).toHaveProperty("repositoryName", repo);
  expect(result.current.other.comments).toHaveLength(0);
});

it("CodeMirror captures touch-only selections and opens the latest quote", () => {
  vi.useFakeTimers();
  const wrapper = document.createElement("div");
  const view = {
    state: EditorState.create({ doc: "old quote", selection: { anchor: 0, head: 3 } }),
    dispatch: vi.fn(),
    coordsAtPos: vi.fn(() => ({ left: 24, right: 28, top: 36, bottom: 54 })),
  };
  const props = {
    path: "README.md",
    sessionId: "s",
    enableComments: true,
    content: "old quote",
    originalContent: "old quote",
    isDirty: false,
    isSaving: false,
    wrapperRef: { current: wrapper },
    editorRef: { current: { view } } as never,
    onChange: vi.fn(),
    onSave: vi.fn(),
  };
  const { result } = renderHook(() => useCodeMirrorEditorState(props));

  act(() => {
    wrapper.dispatchEvent(new Event("touchstart", { bubbles: true }));
    wrapper.dispatchEvent(new Event("touchend", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });
  expect(result.current.currentSelection).toEqual({ text: "old", startLine: 1, endLine: 1 });
  expect(result.current.floatingButtonPos).toBeNull();

  view.state = EditorState.create({
    doc: `first line\n${LATEST_QUOTE}\nlast line`,
    selection: { anchor: 11, head: 23 },
  });
  act(() => {
    wrapper.dispatchEvent(new Event("touchend", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });
  expect(result.current.currentSelection).toEqual({
    text: LATEST_QUOTE,
    startLine: 2,
    endLine: 2,
  });

  act(() => result.current.handleDockButtonClick());
  expect(result.current.textSelection).toEqual({
    text: LATEST_QUOTE,
    startLine: 2,
    endLine: 2,
    position: { x: 24, y: 54 },
  });
});

it("returns CodeMirror touch ownership to a mouse drag inside the same editor", () => {
  vi.useFakeTimers();
  const wrapper = document.createElement("div");
  const editorContent = document.createElement("div");
  wrapper.append(editorContent);
  document.body.append(wrapper);
  const view = {
    state: EditorState.create({ doc: "touch quote", selection: { anchor: 0, head: 5 } }),
    dispatch: vi.fn(),
    coordsAtPos: vi.fn(() => ({ left: 24, right: 28, top: 36, bottom: 54 })),
  };
  const props = {
    path: "README.md",
    sessionId: "s",
    enableComments: true,
    content: "touch quote",
    originalContent: "touch quote",
    isDirty: false,
    isSaving: false,
    wrapperRef: { current: wrapper },
    editorRef: { current: { view } } as never,
    onChange: vi.fn(),
    onSave: vi.fn(),
  };
  const { result } = renderHook(() => useCodeMirrorEditorState(props));

  act(() => {
    editorContent.dispatchEvent(new Event("touchstart", { bubbles: true }));
    editorContent.dispatchEvent(new Event("touchend", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });
  expect(result.current.currentSelection?.text).toBe("touch");

  view.state = EditorState.create({
    doc: `first line\n${LATEST_QUOTE}\nlast line`,
    selection: { anchor: 11, head: 23 },
  });
  act(() => {
    const pointerDown = new Event("pointerdown", { bubbles: true });
    Object.defineProperty(pointerDown, "pointerType", { value: "mouse" });
    editorContent.dispatchEvent(pointerDown);
    editorContent.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    editorContent.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });

  expect(result.current.currentSelection).toEqual({
    text: LATEST_QUOTE,
    startLine: 2,
    endLine: 2,
  });
  expect(result.current.floatingButtonPos).not.toBeNull();
  document.body.removeChild(wrapper);
});

it("Monaco captures touch-only selection changes and opens the matching view zone", async () => {
  let handleSelectionChange: (() => void) | null = null;
  const selection = {
    startLineNumber: 2,
    startColumn: 1,
    endLineNumber: 2,
    endColumn: 13,
    isEmpty: () => false,
  };
  const revealLineInCenter = vi.fn();
  const editor = {
    onDidChangeCursorSelection: (handler: () => void) => {
      handleSelectionChange = handler;
      return { dispose: vi.fn() };
    },
    getSelection: () => selection,
    getModel: () => ({ getValueInRange: () => LATEST_QUOTE }),
    createDecorationsCollection: () => ({ set: vi.fn() }),
    onMouseDown: vi.fn(() => ({ dispose: vi.fn() })),
    addCommand: vi.fn(),
    revealLineInCenter,
  };
  const monaco = {
    KeyMod: { CtrlCmd: 1, Alt: 2 },
    KeyCode: { KeyK: 1, KeyZ: 2 },
  };
  const { result } = renderHook(() =>
    useMonacoEditorComments({
      path: "README.md",
      sessionId: "s",
      enableComments: true,
      wrapperRef: { current: document.createElement("div") },
      contentRef: { current: `first line\n${LATEST_QUOTE}\nlast line` },
      onChange: vi.fn(),
      onSave: vi.fn(),
    }),
  );

  act(() => result.current.handleEditorDidMount(editor as never, monaco as never));
  act(() => handleSelectionChange?.());
  expect(result.current.currentSelection).toEqual({
    text: LATEST_QUOTE,
    startLine: 2,
    endLine: 2,
  });
  act(() => result.current.handleDockButtonClick());
  expect(result.current.formZoneRange).toEqual({
    startLine: 2,
    endLine: 2,
    codeContent: LATEST_QUOTE,
  });
  await act(() => new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve())));
  expect(revealLineInCenter).toHaveBeenCalledWith(2);
});

it.each(["api", ""])("CodeMirror creates and selects comments in scope %j", (repo) => {
  vi.useFakeTimers();
  const wrapper = document.createElement("div");
  const view = {
    state: EditorState.create({ doc: "text", selection: { anchor: 0, head: 4 } }),
    dispatch: vi.fn(),
  };
  const props = {
    path: "README.md",
    sessionId: "s",
    enableComments: true,
    content: "text",
    originalContent: "text",
    isDirty: false,
    isSaving: false,
    wrapperRef: { current: wrapper },
    editorRef: { current: { view } } as never,
    onChange: vi.fn(),
    onSave: vi.fn(),
  };
  const { result } = renderHook(() => ({
    selected: useCodeMirrorEditorState({ ...props, repo }),
    other: useCodeMirrorEditorState({ ...props, repo: "other", wrapperRef: { current: null } }),
  }));
  act(() => {
    wrapper.dispatchEvent(new MouseEvent("mouseup"));
    vi.advanceTimersByTime(10);
  });
  act(() =>
    result.current.selected.handleFloatingButtonClick({
      preventDefault: vi.fn(),
      stopPropagation: vi.fn(),
    } as never),
  );
  act(() => result.current.selected.handleCommentSubmit("Feedback"));
  expect(result.current.selected.comments[0]).toHaveProperty("repositoryName", repo);
  expect(result.current.other.comments).toHaveLength(0);
});
