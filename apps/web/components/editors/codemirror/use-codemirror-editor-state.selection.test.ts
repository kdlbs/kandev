import { act, cleanup, renderHook } from "@testing-library/react";
import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import type { ReactCodeMirrorRef } from "@uiw/react-codemirror";
import { afterEach, expect, it, vi } from "vitest";
import { useCodeMirrorEditorState } from "./use-codemirror-editor-state";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeTaskId: string } }) => unknown) =>
    selector({ tasks: { activeTaskId: "task-1" } }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/lib/commands/command-registry", () => ({
  useCommandPanelOpen: () => ({ setOpen: vi.fn() }),
}));
vi.mock("@/hooks/domains/comments/use-diff-comments", () => ({
  useDiffFileComments: () => [],
}));
vi.mock("@/hooks/domains/comments/use-run-comment", () => ({
  useRunComment: () => ({ runComment: vi.fn() }),
}));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

const wrappers: HTMLElement[] = [];

afterEach(() => {
  cleanup();
  wrappers.splice(0).forEach((wrapper) => wrapper.remove());
  vi.useRealTimers();
});

function setupEditor(readOnly = false) {
  const wrapper = document.createElement("div");
  document.body.append(wrapper);
  wrappers.push(wrapper);
  const wrapperRef = { current: wrapper };
  const editorRef = {
    current: { view: null as unknown as EditorView },
  } as unknown as React.RefObject<ReactCodeMirrorRef | null>;
  const { result } = renderHook(() =>
    useCodeMirrorEditorState({
      path: "README.md",
      content: "first line\nlatest quote\nlast line",
      originalContent: "first line\nlatest quote\nlast line",
      isDirty: false,
      isSaving: false,
      sessionId: "session-1",
      readOnly,
      enableComments: true,
      onChange: vi.fn(),
      onSave: vi.fn(),
      wrapperRef,
      editorRef,
    }),
  );

  const view = new EditorView({
    state: EditorState.create({
      doc: "first line\nlatest quote\nlast line",
      extensions: result.current.extensions,
    }),
    parent: wrapper,
  });
  editorRef.current!.view = view;
  return { wrapper, view, result };
}

it("refreshes the quote after touch selection hands back to a mouse drag", () => {
  const { wrapper, view, result } = setupEditor();
  vi.useFakeTimers();

  act(() => {
    wrapper.dispatchEvent(new Event("touchstart", { bubbles: true }));
    view.dispatch({ selection: { anchor: 0, head: 5 } });
    wrapper.dispatchEvent(new Event("touchend", { bubbles: true }));
  });
  expect(result.current.floatingButtonPos).toBeNull();

  act(() => {
    view.contentDOM.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
  });
  expect(result.current.floatingButtonPos).toBeNull();

  act(() => {
    const pointerDown = new Event("pointerdown", { bubbles: true });
    Object.defineProperty(pointerDown, "pointerType", { value: "mouse" });
    view.contentDOM.dispatchEvent(pointerDown);
    wrapper.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    wrapper.dispatchEvent(new MouseEvent("mousemove", { bubbles: true }));
    view.dispatch({ selection: { anchor: 11, head: 23 } });
    wrapper.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });
  expect(result.current.floatingButtonPos).not.toBeNull();
  act(() => result.current.handleFloatingButtonClick());

  expect(result.current.textSelection?.text).toBe("latest quote");
  view.destroy();
});

it("uses the keyboard-extended quote when the desktop Comment action opens", () => {
  const { wrapper, view, result } = setupEditor();
  vi.useFakeTimers();
  act(() => view.dispatch({ selection: { anchor: 0, head: 5 } }));
  act(() => {
    wrapper.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });
  expect(result.current.floatingButtonPos).not.toBeNull();

  act(() => view.dispatch({ selection: { anchor: 11, head: 23 } }));
  act(() => result.current.handleFloatingButtonClick());
  expect(result.current.textSelection?.text).toBe("latest quote");
  view.destroy();
});

it("dismisses the desktop Comment action when a pointer presses outside the editor", () => {
  const { wrapper, view, result } = setupEditor();
  vi.useFakeTimers();
  act(() => view.dispatch({ selection: { anchor: 0, head: 5 } }));
  act(() => {
    wrapper.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
    vi.advanceTimersByTime(10);
  });
  expect(result.current.floatingButtonPos).not.toBeNull();

  const outside = document.createElement("button");
  document.body.append(outside);
  act(() => {
    const pointerDown = new Event("pointerdown", { bubbles: true });
    Object.defineProperty(pointerDown, "pointerType", { value: "mouse" });
    outside.dispatchEvent(pointerDown);
  });

  expect(result.current.floatingButtonPos).toBeNull();
  outside.remove();
  view.destroy();
});

it("keeps read-only CodeMirror content selectable", () => {
  const { view } = setupEditor(true);

  expect(view.contentDOM.getAttribute("contenteditable")).toBe("false");
  view.dispatch({ selection: { anchor: 0, head: 5 } });
  expect(
    view.state.doc.sliceString(view.state.selection.main.from, view.state.selection.main.to),
  ).toBe("first");
  view.destroy();
});
