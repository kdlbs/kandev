import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { modelUriForDocument } from "@/lib/lsp/file-uri";

const getMonacoInstance = vi.hoisted(() => vi.fn());
const APP_PATH = "src/app.ts";
const MISSING_PATH = "src/missing.ts";
const WORKTREE_PATH = "/worktree";
const WORKTREE_APP_PATH = "/worktree/src/app.ts";
const REPO = "frontend";

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance,
}));

import {
  consumePendingCursorPosition,
  scrollEditorIfMounted,
  setPendingCursorPosition,
  useOpenFileAtLine,
} from "./use-file-editors";

function createEditor(modelPath: string | null, modelUri?: string) {
  return {
    getModel: vi.fn(() =>
      modelPath
        ? {
            uri: {
              path: modelPath,
              toString: () => modelUri ?? `file://${modelPath}`,
            },
          }
        : null,
    ),
    setPosition: vi.fn(),
    revealLineInCenter: vi.fn(),
    focus: vi.fn(),
  };
}

describe("scrollEditorIfMounted", () => {
  afterEach(() => {
    getMonacoInstance.mockReset();
    consumePendingCursorPosition(APP_PATH);
    consumePendingCursorPosition(APP_PATH, REPO);
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
    expect(editor.focus).toHaveBeenCalledTimes(1);
    expect(consumePendingCursorPosition(APP_PATH)).toBeUndefined();
  });

  it("requires the repo segment when repo-scoped fallback scrolling", () => {
    const wrongRepoEditor = createEditor("/worktree/backend/src/app.ts");
    const repoEditor = createEditor("/worktree/frontend/src/app.ts");
    getMonacoInstance.mockReturnValue({
      editor: { getEditors: () => [wrongRepoEditor, repoEditor] },
    });
    setPendingCursorPosition(APP_PATH, 12, 1, REPO);

    expect(scrollEditorIfMounted(APP_PATH, null, 42, 3, { repo: REPO })).toBe(true);

    expect(wrongRepoEditor.setPosition).not.toHaveBeenCalled();
    expect(repoEditor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
    expect(consumePendingCursorPosition(APP_PATH, REPO)).toBeUndefined();
  });

  it("does not suffix-scroll a repo-scoped request into an unscoped model path", () => {
    const editor = createEditor(WORKTREE_APP_PATH);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(APP_PATH, 12, 1, REPO);

    expect(scrollEditorIfMounted(APP_PATH, null, 42, 3, { repo: REPO })).toBe(false);

    expect(editor.setPosition).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(APP_PATH, REPO)).toEqual({ line: 12, column: 1 });
  });

  it("returns false when no mounted Monaco editor matches the path", () => {
    const editor = createEditor("/worktree/src/other.ts");
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(MISSING_PATH, 5, 1);

    expect(scrollEditorIfMounted(MISSING_PATH, WORKTREE_PATH, 5, 1)).toBe(false);

    expect(editor.setPosition).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(MISSING_PATH)).toEqual({ line: 5, column: 1 });
  });

  it("scrolls only the model owned by the requested task session", () => {
    const documentUri = "file:///workspace/src/app.ts";
    const firstModelUri = modelUriForDocument(documentUri, "first-session");
    const secondModelUri = modelUriForDocument(documentUri, "second-session");
    const firstEditor = createEditor(new URL(firstModelUri).pathname, firstModelUri);
    const secondEditor = createEditor(new URL(secondModelUri).pathname, secondModelUri);
    getMonacoInstance.mockReturnValue({
      editor: { getEditors: () => [firstEditor, secondEditor] },
    });

    expect(
      scrollEditorIfMounted(APP_PATH, "file:///workspace", 42, 3, {
        sessionId: "second-session",
      }),
    ).toBe(true);

    expect(firstEditor.setPosition).not.toHaveBeenCalled();
    expect(secondEditor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
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
