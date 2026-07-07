import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const getMonacoInstance = vi.hoisted(() => vi.fn());
const APP_PATH = "src/app.ts";
const MISSING_PATH = "src/missing.ts";
const WORKTREE_PATH = "/worktree";
const WORKTREE_APP_PATH = "/worktree/src/app.ts";

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance,
}));

import {
  consumePendingCursorPosition,
  scrollEditorIfMounted,
  setPendingCursorPosition,
  useOpenFileAtLine,
} from "./use-file-editors";

function createEditor(modelPath: string | null) {
  return {
    getModel: vi.fn(() => (modelPath ? { uri: { path: modelPath } } : null)),
    setPosition: vi.fn(),
    revealLineInCenter: vi.fn(),
    focus: vi.fn(),
  };
}

describe("scrollEditorIfMounted", () => {
  afterEach(() => {
    getMonacoInstance.mockReset();
    consumePendingCursorPosition(APP_PATH);
    consumePendingCursorPosition(MISSING_PATH);
  });

  it("scrolls the mounted Monaco editor that matches the worktree path", () => {
    const editor = createEditor(WORKTREE_APP_PATH);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(APP_PATH, 12, 1);

    expect(scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 42, 3)).toBe(true);

    expect(editor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
    expect(editor.revealLineInCenter).toHaveBeenCalledWith(42);
    expect(editor.focus).toHaveBeenCalledTimes(1);
    expect(consumePendingCursorPosition(APP_PATH)).toBeUndefined();
  });

  it("falls back to a path-segment suffix match when the worktree path is unknown", () => {
    const editor = createEditor(WORKTREE_APP_PATH);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(APP_PATH, 12, 1);

    expect(scrollEditorIfMounted(APP_PATH, null, 42, 3)).toBe(true);

    expect(editor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
    expect(editor.revealLineInCenter).toHaveBeenCalledWith(42);
    expect(consumePendingCursorPosition(APP_PATH)).toBeUndefined();
  });

  it("returns false when no mounted Monaco editor matches the path", () => {
    const editor = createEditor("/worktree/src/other.ts");
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(MISSING_PATH, 5, 1);

    expect(scrollEditorIfMounted(MISSING_PATH, WORKTREE_PATH, 5, 1)).toBe(false);

    expect(editor.setPosition).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(MISSING_PATH)).toEqual({ line: 5, column: 1 });
  });
});

describe("useOpenFileAtLine", () => {
  afterEach(() => {
    getMonacoInstance.mockReset();
    consumePendingCursorPosition(APP_PATH);
  });

  it("opens and scrolls an already-mounted file when a target line is present", () => {
    const openFile = vi.fn();
    const editor = createEditor(WORKTREE_APP_PATH);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    const { result } = renderHook(() => useOpenFileAtLine(openFile, 88, WORKTREE_PATH));

    act(() => result.current(APP_PATH));

    expect(openFile).toHaveBeenCalledWith(APP_PATH);
    expect(editor.setPosition).toHaveBeenCalledWith({ lineNumber: 88, column: 1 });
    expect(consumePendingCursorPosition(APP_PATH)).toBeUndefined();
  });

  it("opens without setting a pending position when the target line is missing", () => {
    const openFile = vi.fn();
    const { result } = renderHook(() => useOpenFileAtLine(openFile, undefined, WORKTREE_PATH));

    act(() => result.current(APP_PATH));

    expect(openFile).toHaveBeenCalledWith(APP_PATH);
    expect(getMonacoInstance).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(APP_PATH)).toBeUndefined();
  });
});
