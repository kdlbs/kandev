import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { EditorState } from "@codemirror/state";
import { useCommentsStore } from "@/lib/state/slices/comments";
import { useMonacoEditorComments } from "./monaco/use-monaco-editor-state";
import { useCodeMirrorEditorState } from "./codemirror/use-codemirror-editor-state";

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
