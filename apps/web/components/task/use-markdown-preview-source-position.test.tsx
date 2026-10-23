import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { SourcePositionHandle } from "@/components/editors/source-position";
import { useMarkdownPreviewSourcePosition } from "./use-markdown-preview-source-position";

function preview() {
  const scroll = document.createElement("div");
  const root = document.createElement("div");
  scroll.append(root);
  root.innerHTML =
    '<p data-md-source-start="1" data-md-source-end="2">First</p><p data-md-source-start="8" data-md-source-end="9">Last</p>';
  scroll.getBoundingClientRect = () => ({ top: 50, bottom: 250 }) as DOMRect;
  const blocks = root.querySelectorAll("p");
  blocks[0].getBoundingClientRect = () => ({ top: -100, bottom: -50 }) as DOMRect;
  blocks[1].getBoundingClientRect = () => ({ top: 80, bottom: 120 }) as DOMRect;
  return { rootRef: { current: root }, scrollRef: { current: scroll } };
}

describe("Preview source position handoff (AC-UI-MARKDOWN-FILE-EDITING-002.4)", () => {
  it("retains source selection intent and scrolls only its own mapped container", () => {
    const refs = preview();
    const callback = vi.fn();
    const content = Array.from({ length: 12 }, (_, i) => `Line ${i}`).join("\n");
    renderHook(() =>
      useMarkdownPreviewSourcePosition({
        ...refs,
        identity: "file",
        content,
        onSourcePositionHandle: callback,
      }),
    );
    const handle = callback.mock.lastCall![0] as SourcePositionHandle;
    expect(handle.capture()).toEqual({ nearestLine: 8 });
    handle.restore({ selection: { anchor: 63, active: 50 }, nearestLine: 8 });
    expect(refs.scrollRef.current.scrollTop).toBe(30);
    expect(handle.capture()).toEqual({ nearestLine: 8, selection: { anchor: 63, active: 50 } });
  });

  it("invalidates a stale handle when switching files or hiding, and does not scroll on typing", () => {
    const refs = preview();
    const callback = vi.fn();
    const { rerender } = renderHook(
      ({ identity, content, active }) =>
        useMarkdownPreviewSourcePosition({
          ...refs,
          identity,
          content,
          isActive: active,
          onSourcePositionHandle: callback,
        }),
      {
        initialProps: { identity: "first", content: "one\ntwo", active: true },
      },
    );
    const stale = callback.mock.lastCall![0] as SourcePositionHandle;
    rerender({ identity: "first", content: "one\ntwo!", active: true });
    expect(callback).toHaveBeenCalledTimes(1);
    expect(refs.scrollRef.current.scrollTop).toBe(0);
    rerender({ identity: "second", content: "x", active: true });
    stale.restore({ nearestLine: 8 });
    expect(refs.scrollRef.current.scrollTop).toBe(0);
    const current = callback.mock.lastCall![0] as SourcePositionHandle;
    current.restore({ selection: { anchor: 99, active: 0 }, nearestLine: 8 });
    expect(current.capture()!.selection).toEqual({ anchor: 1, active: 0 });
    rerender({ identity: "second", content: "x", active: false });
    refs.scrollRef.current.scrollTop = 0;
    current.restore({ nearestLine: 1 });
    expect(refs.scrollRef.current.scrollTop).toBe(0);
  });
});
