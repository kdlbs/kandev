import { act, renderHook } from "@testing-library/react";
import type { editor as monacoEditor } from "monaco-editor";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  consumePendingCursorPosition,
  EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS,
  setPendingCursorPosition,
} from "@/hooks/file-editor-cursor";
import { useMonacoCursorNavigation } from "./use-monaco-cursor-navigation";

const TARGET_PATH = "src/target.ts";
const OTHER_PATH = "src/other.ts";
const WORKTREE_PATH = "/worktree";

function fileModel(path: string, id: string) {
  const filePath = `${WORKTREE_PATH}/${path}`;
  return {
    id,
    uri: {
      path: filePath,
      toString: () => `file://${filePath}`,
    },
  };
}

function createModelSwitchingEditor() {
  let model = fileModel(OTHER_PATH, "other-model");
  const modelListeners = new Set<() => void>();
  const decorationCollections: Array<{
    initial: monacoEditor.IModelDeltaDecoration[];
    set: ReturnType<typeof vi.fn>;
  }> = [];
  const editor = {
    createDecorationsCollection: vi.fn((initial: monacoEditor.IModelDeltaDecoration[]) => {
      const collection = { initial, set: vi.fn() };
      decorationCollections.push(collection);
      return collection;
    }),
    focus: vi.fn(),
    getModel: () => model,
    layout: vi.fn(),
    onDidChangeModel: (listener: () => void) => {
      modelListeners.add(listener);
      return { dispose: () => modelListeners.delete(listener) };
    },
    revealLineInCenter: vi.fn(),
    setPosition: vi.fn(),
  } as unknown as monacoEditor.IStandaloneCodeEditor;

  return {
    decorationCollections,
    editor,
    switchToTarget() {
      model = fileModel(TARGET_PATH, "target-model");
      for (const listener of modelListeners) listener();
    },
  };
}

afterEach(() => {
  consumePendingCursorPosition(TARGET_PATH);
  vi.useRealTimers();
});

describe("useMonacoCursorNavigation", () => {
  it("reveals a flashed pending cursor after a cached Monaco model swap", () => {
    vi.useFakeTimers();
    const fake = createModelSwitchingEditor();
    const { rerender } = renderHook(
      ({ path }) =>
        useMonacoCursorNavigation({
          editor: fake.editor,
          path,
          worktreePath: WORKTREE_PATH,
        }),
      { initialProps: { path: OTHER_PATH } },
    );
    setPendingCursorPosition(TARGET_PATH, 180, 7, undefined, { flashLine: true });

    rerender({ path: TARGET_PATH });
    expect(fake.editor.setPosition).not.toHaveBeenCalled();
    act(() => fake.switchToTarget());

    expect(fake.editor.layout).toHaveBeenCalledTimes(1);
    expect(fake.editor.setPosition).toHaveBeenCalledWith({ lineNumber: 180, column: 7 });
    expect(fake.editor.revealLineInCenter).toHaveBeenCalledWith(180);
    expect(fake.decorationCollections[0].initial).toEqual([
      expect.objectContaining({
        range: expect.objectContaining({ startLineNumber: 180, endLineNumber: 180 }),
        options: expect.objectContaining({
          isWholeLine: true,
          className: "editor-content-search-flash",
        }),
      }),
    ]);
    expect(consumePendingCursorPosition(TARGET_PATH)).toBeUndefined();

    act(() => vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS));
    expect(fake.decorationCollections[0].set).toHaveBeenLastCalledWith([]);
  });
});
