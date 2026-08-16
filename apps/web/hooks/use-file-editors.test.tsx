import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { modelUriForDocument } from "@/lib/lsp/file-uri";

const getMonacoInstance = vi.hoisted(() => vi.fn());
const APP_PATH = "src/app.ts";
const ENCODED_APP_PATH = "src/app #1.ts";
const MISSING_PATH = "src/missing.ts";
const WORKTREE_PATH = "/worktree";
const WORKTREE_APP_PATH = "/worktree/src/app.ts";
const WORKSPACE_URI = "file:///workspace";
const DOCUMENT_URI = `${WORKSPACE_URI}/${APP_PATH}`;
const REPO = "frontend";
const FIRST_SESSION_ID = "first-session";
const SECOND_SESSION_ID = "second-session";

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance,
}));

import {
  consumePendingCursorPosition,
  scrollEditorIfMounted,
  setPendingCursorPosition,
  useOpenFileAtLine,
} from "./use-file-editors";
import { EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS } from "./file-editor-cursor";

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
    layout: vi.fn(),
    setPosition: vi.fn(),
    revealLineInCenter: vi.fn(),
    focus: vi.fn(),
  };
}

afterEach(() => {
  getMonacoInstance.mockReset();
  consumePendingCursorPosition(APP_PATH);
  consumePendingCursorPosition(APP_PATH, REPO);
  consumePendingCursorPosition(MISSING_PATH);
  vi.useRealTimers();
});

describe("scrollEditorIfMounted", () => {
  it("scrolls the mounted Monaco editor that matches the worktree path", () => {
    const editor = createEditor(WORKTREE_APP_PATH);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(APP_PATH, 12, 1);

    expect(scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 42, 3)).toBe(true);

    expect(editor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
    expect(editor.revealLineInCenter).toHaveBeenCalledWith(42);
    expect(editor.layout).not.toHaveBeenCalled();
    expect(editor.focus).toHaveBeenCalledTimes(1);
    expect(consumePendingCursorPosition(APP_PATH)).toBeUndefined();
  });
});

describe("scrollEditorIfMounted flashes", () => {
  it("adds a one-shot whole-line decoration for a flashed reveal", () => {
    const editor = {
      ...createEditor(WORKTREE_APP_PATH),
      createDecorationsCollection: vi.fn(() => ({ set: vi.fn() })),
    };
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(APP_PATH, 42, 3);

    expect(
      scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 42, 3, {
        flashLine: true,
      }),
    ).toBe(true);

    expect(editor.createDecorationsCollection).toHaveBeenCalledWith([
      expect.objectContaining({
        range: expect.objectContaining({ startLineNumber: 42, endLineNumber: 42 }),
        options: expect.objectContaining({
          isWholeLine: true,
          className: "editor-content-search-flash",
        }),
      }),
    ]);
  });

  it("keeps a restarted Monaco flash until the latest request expires", () => {
    vi.useFakeTimers();
    const collections = [vi.fn(), vi.fn()];
    const editor = {
      ...createEditor(WORKTREE_APP_PATH),
      createDecorationsCollection: vi
        .fn()
        .mockImplementationOnce(() => ({ set: collections[0] }))
        .mockImplementationOnce(() => ({ set: collections[1] })),
    };
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });

    scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 42, 3, { flashLine: true });
    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS / 2);
    scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 43, 1, { flashLine: true });
    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS / 2);

    expect(editor.createDecorationsCollection).toHaveBeenCalledTimes(2);
    expect(collections[0]).toHaveBeenLastCalledWith([]);
    expect(collections[1]).not.toHaveBeenCalled();

    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS / 2);
    expect(collections[1]).toHaveBeenLastCalledWith([]);
  });
});

describe("scrollEditorIfMounted matching", () => {
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

  it("uses repo-scoped matching when a worktree path is also available", () => {
    const wrongRepoEditor = createEditor("/worktree/backend/src/app.ts");
    const repoEditor = createEditor("/worktree/frontend/src/app.ts");
    getMonacoInstance.mockReturnValue({
      editor: { getEditors: () => [wrongRepoEditor, repoEditor] },
    });
    setPendingCursorPosition(APP_PATH, 12, 1, REPO);

    expect(scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 42, 3, { repo: REPO })).toBe(true);

    expect(wrongRepoEditor.setPosition).not.toHaveBeenCalled();
    expect(repoEditor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
    expect(consumePendingCursorPosition(APP_PATH, REPO)).toBeUndefined();
  });

  it("does not scroll a repo-scoped request in an unscoped worktree model", () => {
    const editor = createEditor(WORKTREE_APP_PATH);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });
    setPendingCursorPosition(APP_PATH, 12, 1, REPO);

    expect(scrollEditorIfMounted(APP_PATH, WORKTREE_PATH, 42, 3, { repo: REPO })).toBe(false);

    expect(editor.setPosition).not.toHaveBeenCalled();
    expect(consumePendingCursorPosition(APP_PATH, REPO)).toEqual({ line: 12, column: 1 });
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
});

describe("scrollEditorIfMounted session model matching", () => {
  afterEach(() => getMonacoInstance.mockReset());

  it("scrolls only the model owned by the requested task session", () => {
    const firstModelUri = modelUriForDocument(DOCUMENT_URI, FIRST_SESSION_ID);
    const secondModelUri = modelUriForDocument(DOCUMENT_URI, SECOND_SESSION_ID);
    const firstEditor = createEditor(new URL(firstModelUri).pathname, firstModelUri);
    const secondEditor = createEditor(new URL(secondModelUri).pathname, secondModelUri);
    getMonacoInstance.mockReturnValue({
      editor: { getEditors: () => [firstEditor, secondEditor] },
    });

    expect(
      scrollEditorIfMounted(APP_PATH, WORKSPACE_URI, 42, 3, {
        sessionId: SECOND_SESSION_ID,
      }),
    ).toBe(true);

    expect(firstEditor.setPosition).not.toHaveBeenCalled();
    expect(secondEditor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
  });

  it("falls back to the sole session-qualified model for an unscoped caller", () => {
    const documentUri = `${WORKSPACE_URI}/src/app%20%231.ts`;
    const modelUri = modelUriForDocument(documentUri, FIRST_SESSION_ID);
    const editor = createEditor(new URL(modelUri).pathname, modelUri);
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [editor] } });

    expect(scrollEditorIfMounted(ENCODED_APP_PATH, WORKSPACE_URI, 42, 3)).toBe(true);

    expect(editor.setPosition).toHaveBeenCalledWith({ lineNumber: 42, column: 3 });
    expect(editor.revealLineInCenter).toHaveBeenCalledWith(42);
    expect(editor.focus).toHaveBeenCalledTimes(1);
  });

  it("does not choose between session-qualified models for an unscoped caller", () => {
    const firstModelUri = modelUriForDocument(DOCUMENT_URI, FIRST_SESSION_ID);
    const secondModelUri = modelUriForDocument(DOCUMENT_URI, SECOND_SESSION_ID);
    const firstEditor = createEditor(new URL(firstModelUri).pathname, firstModelUri);
    const secondEditor = createEditor(new URL(secondModelUri).pathname, secondModelUri);
    getMonacoInstance.mockReturnValue({
      editor: { getEditors: () => [firstEditor, secondEditor] },
    });

    expect(scrollEditorIfMounted(APP_PATH, WORKSPACE_URI, 42, 3)).toBe(false);

    expect(firstEditor.setPosition).not.toHaveBeenCalled();
    expect(secondEditor.setPosition).not.toHaveBeenCalled();
  });
});

describe("useOpenFileAtLine", () => {
  afterEach(() => {
    getMonacoInstance.mockReset();
    consumePendingCursorPosition(APP_PATH);
    consumePendingCursorPosition(APP_PATH, undefined, FIRST_SESSION_ID);
    consumePendingCursorPosition(APP_PATH, undefined, SECOND_SESSION_ID);
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

  it("scrolls only the requested session-qualified model", () => {
    const firstModelUri = modelUriForDocument(DOCUMENT_URI, FIRST_SESSION_ID);
    const secondModelUri = modelUriForDocument(DOCUMENT_URI, SECOND_SESSION_ID);
    const firstEditor = createEditor(new URL(firstModelUri).pathname, firstModelUri);
    const secondEditor = createEditor(new URL(secondModelUri).pathname, secondModelUri);
    getMonacoInstance.mockReturnValue({
      editor: { getEditors: () => [firstEditor, secondEditor] },
    });
    const { result } = renderHook(() =>
      useOpenFileAtLine(vi.fn(), 88, WORKSPACE_URI, SECOND_SESSION_ID),
    );

    act(() => result.current(APP_PATH));

    expect(firstEditor.setPosition).not.toHaveBeenCalled();
    expect(secondEditor.setPosition).toHaveBeenCalledWith({ lineNumber: 88, column: 1 });
  });

  it("keeps a delayed-mount cursor scoped to the requested session", () => {
    getMonacoInstance.mockReturnValue({ editor: { getEditors: () => [] } });
    const { result } = renderHook(() =>
      useOpenFileAtLine(vi.fn(), 88, "file:///workspace", SECOND_SESSION_ID),
    );

    act(() => result.current(APP_PATH));

    expect(consumePendingCursorPosition(APP_PATH, undefined, FIRST_SESSION_ID)).toBeUndefined();
    expect(consumePendingCursorPosition(APP_PATH, undefined, SECOND_SESSION_ID)).toEqual({
      line: 88,
      column: 1,
    });
  });

  it("lets a session mount consume a legacy unscoped cursor", () => {
    setPendingCursorPosition(APP_PATH, 27, 2);

    expect(consumePendingCursorPosition(APP_PATH, undefined, SECOND_SESSION_ID)).toEqual({
      line: 27,
      column: 2,
    });
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
