import { createRef } from "react";
import { act, renderHook } from "@testing-library/react";
import type { Editor } from "@tiptap/core";
import { describe, expect, it, vi } from "vitest";
import { usePlanFindShortcut } from "./use-plan-find-shortcut";

type EditorDouble = Editor & { isDestroyed: boolean };

function destroyedEditor(): EditorDouble {
  return {
    isDestroyed: true,
    on: vi.fn(),
    off: vi.fn(),
    get commands() {
      throw new Error("destroyed editor commands were read");
    },
  } as unknown as EditorDouble;
}

function liveEditor(): EditorDouble {
  return {
    isDestroyed: false,
    on: vi.fn(),
    off: vi.fn(),
    commands: {
      clearPlanSearch: vi.fn(),
      setPlanSearchQuery: vi.fn(),
      planSearchNext: vi.fn(),
      planSearchPrev: vi.fn(),
    },
  } as unknown as EditorDouble;
}

describe("usePlanFindShortcut editor lifecycle", () => {
  // @covers AC-UI-PLAN-EDITOR-TASK-SWITCH-001.2
  it("does not read commands from an editor that is already destroyed", () => {
    const editor = destroyedEditor();

    expect(() =>
      renderHook(() => usePlanFindShortcut(createRef<HTMLDivElement>(), editor)),
    ).not.toThrow();
    expect(editor.on).not.toHaveBeenCalled();
  });

  // @covers AC-UI-PLAN-EDITOR-TASK-SWITCH-001.3
  it("does not use an editor that is destroyed after the hook mounts", () => {
    const editor = liveEditor();
    const view = renderHook(() => usePlanFindShortcut(createRef<HTMLDivElement>(), editor));
    const off = editor.off;

    act(() => view.result.current.open());
    editor.isDestroyed = true;

    Object.defineProperty(editor, "commands", {
      get() {
        throw new Error("destroyed editor commands were read");
      },
    });

    expect(() => {
      act(() => view.result.current.setQuery("fox"));
      act(() => view.result.current.close());
      act(() => view.result.current.findNext());
      view.unmount();
    }).not.toThrow();
    expect(off).not.toHaveBeenCalled();
  });
});
