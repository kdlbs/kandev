import { act, renderHook } from "@testing-library/react";
import { EditorState, type TransactionSpec } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";
import { describe, expect, it, vi } from "vitest";
import type { SourcePositionHandle } from "./source-position";
import { useCodeMirrorSourcePosition } from "./use-source-position";

function codeMirror(content: string, anchor = 0, head = 0) {
  let state = EditorState.create({ doc: content, selection: { anchor, head } });
  const view = {
    get state() {
      return state;
    },
    scrollDOM: { getBoundingClientRect: () => ({ top: 0 }) },
    lineBlockAtHeight: () => ({ from: 0 }),
    documentTop: 0,
    dispatch: vi.fn((spec: TransactionSpec) => {
      state = state.update(spec).state;
    }),
    focus: vi.fn(),
  } as unknown as EditorView;
  return view;
}

describe("source provider selection handoff (AC-UI-MARKDOWN-FILE-EDITING-002.4)", () => {
  it("transfers reversed selection through normalized CodeMirror CRLF source offsets", () => {
    const view = codeMirror("one\ntwo\nthree", 10, 5);
    let handle: SourcePositionHandle | null = null;
    const onSourcePositionHandle = (next: SourcePositionHandle | null) => {
      handle = next;
    };
    renderHook(() =>
      useCodeMirrorSourcePosition({ view, content: "one\r\ntwo\r\nthree", onSourcePositionHandle }),
    );
    expect(handle!.capture()).toEqual({ selection: { anchor: 12, active: 6 }, nearestLine: 2 });
    act(() => handle!.restore({ selection: { anchor: 12, active: 6 }, nearestLine: 3 }));
    expect(view.state.selection.main.anchor).toBe(10);
    expect(view.state.selection.main.head).toBe(5);
  });

  it("registers only when active and ready, and stale handles cannot restore after hide", () => {
    const onSourcePositionHandle = vi.fn();
    const view = codeMirror("one\ntwo");
    const { rerender, unmount } = renderHook(
      ({ active, ready }) =>
        useCodeMirrorSourcePosition({
          view: ready ? view : null,
          content: "one\ntwo",
          isActive: active,
          onSourcePositionHandle,
        }),
      { initialProps: { active: false, ready: false } },
    );
    rerender({ active: false, ready: true });
    expect(onSourcePositionHandle).not.toHaveBeenCalled();
    rerender({ active: true, ready: true });
    const handle = onSourcePositionHandle.mock.lastCall![0] as SourcePositionHandle;
    handle.restore({ nearestLine: 2 });
    expect(view.state.selection.main.head).toBe(4);
    rerender({ active: false, ready: true });
    vi.mocked(view.dispatch).mockClear();
    handle.restore({ nearestLine: 1 });
    expect(view.dispatch).not.toHaveBeenCalled();
    expect(onSourcePositionHandle).toHaveBeenLastCalledWith(null);
    unmount();
  });

  it("uses accepted replacement content without restoring on ordinary typing", () => {
    const view = codeMirror("one\ntwo");
    const onSourcePositionHandle = vi.fn();
    const { rerender } = renderHook(
      ({ content }) => useCodeMirrorSourcePosition({ view, content, onSourcePositionHandle }),
      {
        initialProps: { content: "one\ntwo" },
      },
    );
    const handle = onSourcePositionHandle.mock.lastCall![0] as SourcePositionHandle;
    rerender({ content: "one\ntwo!" });
    expect(view.dispatch).not.toHaveBeenCalled();
    expect(onSourcePositionHandle).toHaveBeenCalledTimes(1);
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: "x" } });
    rerender({ content: "x" });
    handle.restore({ selection: { anchor: 99, active: 0 }, nearestLine: 99 });
    expect(view.state.selection.main.anchor).toBe(1);
    expect(view.state.selection.main.head).toBe(0);
  });

  it("retains a scrolled reading line when the collapsed caret remains above the viewport", () => {
    const view = codeMirror("one\ntwo\nthree");
    view.lineBlockAtHeight = () => ({ from: 8 }) as ReturnType<EditorView["lineBlockAtHeight"]>;
    const callback = vi.fn();
    renderHook(() =>
      useCodeMirrorSourcePosition({
        view,
        content: "one\ntwo\nthree",
        onSourcePositionHandle: callback,
      }),
    );
    const handle = callback.mock.lastCall![0] as SourcePositionHandle;
    expect(handle.capture()).toEqual({ selection: { anchor: 0, active: 0 }, nearestLine: 3 });
    handle.restore({ selection: { anchor: 0, active: 0 }, nearestLine: 3 });
    const effects = vi.mocked(view.dispatch).mock.lastCall![0].effects;
    expect(effects).toMatchObject({ value: { range: { head: 8 } } });
  });
});
