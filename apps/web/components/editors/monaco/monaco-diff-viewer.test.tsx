import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const editorHarness = vi.hoisted(() => ({
  onRestore: vi.fn(),
  onSave: vi.fn(() => ({ modified: { scrollTop: 240 }, original: { scrollTop: 120 } })),
  onLayout: vi.fn(),
  onSetScrollTop: vi.fn((value: number) => {
    editorHarness.scrollTop = value;
  }),
  onSetSelection: vi.fn(),
  modelChangeListeners: [] as Array<() => void>,
  modifiedContent: "",
  originalContent: "before\n",
  scrollTop: 240,
  visibleLine: 60,
  selection: null as null | {
    selectionStartLineNumber: number;
    selectionStartColumn: number;
    positionLineNumber: number;
    positionColumn: number;
  },
}));

vi.mock("@monaco-editor/react", async () => {
  const React = await import("react");
  const modifiedEditor = {
    layout: editorHarness.onLayout,
    getValue: () => editorHarness.modifiedContent,
    onDidChangeModelContent: (listener: () => void) => {
      editorHarness.modelChangeListeners.push(listener);
      return {
        dispose: () => {
          editorHarness.modelChangeListeners = editorHarness.modelChangeListeners.filter(
            (current) => current !== listener,
          );
        },
      };
    },
    getModel: () => ({
      getLineCount: () => editorHarness.modifiedContent.split("\n").length,
      getLinesContent: () => editorHarness.modifiedContent.split("\n"),
      getLineContent: (line: number) => editorHarness.modifiedContent.split("\n")[line - 1] ?? "",
      getLineMaxColumn: (line: number) =>
        (editorHarness.modifiedContent.split("\n")[line - 1] ?? "").length + 1,
    }),
    getVisibleRanges: () => [
      { startLineNumber: editorHarness.visibleLine, endLineNumber: editorHarness.visibleLine + 10 },
    ],
    getScrollHeight: () => editorHarness.modifiedContent.split("\n").length * 10,
    getLayoutInfo: () => ({ height: 300 }),
    getTopForPosition: (line: number) => line * 10,
    getScrollTop: () => editorHarness.scrollTop,
    setScrollTop: editorHarness.onSetScrollTop,
    getSelection: () => editorHarness.selection,
    setSelection: editorHarness.onSetSelection,
  };
  const originalEditor = {
    layout: editorHarness.onLayout,
    getValue: () => editorHarness.originalContent,
    getModel: () => ({
      getLineCount: () => editorHarness.originalContent.split("\n").length,
      getLineContent: (line: number) => editorHarness.originalContent.split("\n")[line - 1] ?? "",
      getLineMaxColumn: (line: number) =>
        (editorHarness.originalContent.split("\n")[line - 1] ?? "").length + 1,
    }),
    getVisibleRanges: () => [{ startLineNumber: 1, endLineNumber: 1 }],
    getTopForPosition: (line: number) => line * 10,
    getScrollTop: () => 0,
    setScrollTop: editorHarness.onSetScrollTop,
    getSelection: () => null,
    setSelection: editorHarness.onSetSelection,
    onDidChangeModelContent: () => ({ dispose: vi.fn() }),
  };
  const editor = {
    saveViewState: editorHarness.onSave,
    restoreViewState: editorHarness.onRestore,
    getModifiedEditor: () => modifiedEditor,
    getOriginalEditor: () => originalEditor,
  };
  return {
    DiffEditor: ({ onMount, original, modified }: Record<string, unknown>) => {
      React.useEffect(() => {
        const nextContent = String(modified ?? "");
        if (editorHarness.modifiedContent === nextContent) return;
        editorHarness.modifiedContent = nextContent;
        for (const listener of editorHarness.modelChangeListeners) listener();
      }, [modified]);
      React.useLayoutEffect(() => {
        (onMount as (value: typeof editor) => void)(editor);
      }, [onMount]);
      return React.createElement("div", {
        "data-testid": "monaco-model",
        "data-original": original,
        "data-modified": modified,
      });
    },
  };
});
vi.mock("@/components/theme/app-theme", () => ({ useTheme: () => ({ resolvedTheme: "dark" }) }));
vi.mock("@/lib/commands/command-registry", () => ({
  useCommandPanelOpen: () => ({ setOpen: vi.fn() }),
}));
vi.mock("@/hooks/use-diff-editor-height", () => ({ useDiffEditorHeight: () => 300 }));
vi.mock("@/hooks/use-global-view-mode", () => ({
  useGlobalViewMode: () => ["unified", vi.fn()],
}));
vi.mock("@/hooks/use-capture-keydown", () => ({ useCaptureKeydown: vi.fn() }));
vi.mock("./use-global-folding", () => ({ useGlobalFolding: () => [false, vi.fn()] }));
vi.mock("./monaco-init", () => ({ initMonacoThemes: vi.fn() }));
vi.mock("./use-diff-viewer-comments", async () => {
  const React = await import("react");
  return {
    useDiffViewerComments: () => {
      const diffEditorRef = React.useRef<unknown>(null);
      const [modifiedEditor, setModifiedEditor] = React.useState<object | null>(null);
      const handleDiffEditorMount = React.useCallback((editor: unknown) => {
        diffEditorRef.current = editor;
        setModifiedEditor({});
      }, []);
      return {
        diffEditorRef,
        modifiedEditor,
        originalEditor: null,
        contextMenu: null,
        setContextMenu: vi.fn(),
        handleDiffEditorMount,
        copyAllChangedLines: vi.fn(),
      };
    },
  };
});
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { MonacoDiffViewer } from "./monaco-diff-viewer";

function data(value: string) {
  return {
    filePath: "src/app.ts",
    oldContent: "before\n",
    newContent: value,
    diff: `@@ -1 +1 @@\n-before\n+${value.trimEnd()}`,
    additions: 1,
    deletions: 1,
  };
}

let frames: Map<number, FrameRequestCallback>;
let nextFrame: number;

function renderViewer(content: string, key = "workspace-target") {
  return render(
    <MonacoDiffViewer key={key} data={data(content)} compact hideHeader sessionId="session-1" />,
  );
}

function flushFrames() {
  act(() => {
    for (const [id, callback] of frames) {
      frames.delete(id);
      callback(0);
    }
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  editorHarness.onRestore.mockClear();
  editorHarness.onSave.mockClear();
  editorHarness.onLayout.mockClear();
  editorHarness.onSetScrollTop.mockClear();
  editorHarness.onSetSelection.mockClear();
  editorHarness.modifiedContent = "";
  editorHarness.originalContent = "before\n";
  editorHarness.modelChangeListeners = [];
  editorHarness.scrollTop = 240;
  editorHarness.visibleLine = 60;
  editorHarness.selection = null;
});

beforeEach(() => {
  frames = new Map();
  nextFrame = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    const id = ++nextFrame;
    frames.set(id, callback);
    return id;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
});

describe("Monaco diff refresh continuity", () => {
  it("keeps identical content in place and restores saved view state after changed content", () => {
    const view = renderViewer("after\n");
    view.rerender(
      <MonacoDiffViewer
        key="workspace-target"
        data={data("after\n")}
        compact
        hideHeader
        sessionId="session-1"
      />,
    );
    expect(editorHarness.onSave).not.toHaveBeenCalled();
    expect(editorHarness.onRestore).not.toHaveBeenCalled();

    view.rerender(
      <MonacoDiffViewer
        key="workspace-target"
        data={data("after updated\n")}
        compact
        hideHeader
        sessionId="session-1"
      />,
    );
    expect(editorHarness.onSave).toHaveBeenCalledOnce();
    expect(editorHarness.onRestore).not.toHaveBeenCalled();
    flushFrames();
    expect(editorHarness.onRestore).toHaveBeenCalledWith({
      modified: { scrollTop: 240 },
      original: { scrollTop: 120 },
    });
    expect(editorHarness.onLayout).toHaveBeenCalledOnce();
  });

  it("maps Monaco's visible and selected content lines when changed lines shift them", () => {
    const originalLines = Array.from({ length: 100 }, (_, index) => `stable-line-${index + 1}`);
    const updatedLines = [...originalLines];
    updatedLines.splice(19, 4);
    updatedLines.splice(
      30,
      0,
      ...Array.from({ length: 12 }, (_, index) => `inserted-${index + 1}`),
    );

    const view = renderViewer(`${originalLines.join("\n")}\n`);
    editorHarness.selection = {
      selectionStartLineNumber: 62,
      selectionStartColumn: 3,
      positionLineNumber: 62,
      positionColumn: 6,
    };
    view.rerender(
      <MonacoDiffViewer
        key="workspace-target"
        data={data(`${updatedLines.join("\n")}\n`)}
        compact
        hideHeader
        sessionId="session-1"
      />,
    );
    flushFrames();

    expect(editorHarness.onRestore).toHaveBeenCalledOnce();
    expect(editorHarness.onSetScrollTop).toHaveBeenCalledWith(320);
    expect(editorHarness.onSetScrollTop).toHaveBeenCalledTimes(1);
    expect(editorHarness.onSetSelection).toHaveBeenCalledWith({
      selectionStartLineNumber: 70,
      selectionStartColumn: 3,
      positionLineNumber: 70,
      positionColumn: 6,
    });
  });
});

describe("Monaco diff refresh fallbacks", () => {
  it("maps a removed Monaco anchor to nearby surviving content", () => {
    const originalLines = Array.from({ length: 100 }, (_, index) => `stable-line-${index + 1}`);
    const updatedLines = [...originalLines];
    updatedLines.splice(59, 1);
    updatedLines.splice(
      30,
      0,
      ...Array.from({ length: 10 }, (_, index) => `inserted-${index + 1}`),
    );

    const view = renderViewer(`${originalLines.join("\n")}\n`);
    view.rerender(
      <MonacoDiffViewer
        key="workspace-target"
        data={data(`${updatedLines.join("\n")}\n`)}
        compact
        hideHeader
        sessionId="session-1"
      />,
    );
    flushFrames();

    expect(editorHarness.onSetScrollTop).toHaveBeenCalledWith(330);
    expect(editorHarness.onSetScrollTop).toHaveBeenCalledTimes(1);
  });

  it("does not restore across target replacement or after the user scrolls", () => {
    const view = renderViewer("before\n");
    view.rerender(
      <MonacoDiffViewer
        key="workspace-target"
        data={data("changed\n")}
        compact
        hideHeader
        sessionId="session-1"
      />,
    );
    fireEvent.wheel(view.container.querySelector(".monaco-diff-viewer")!);
    flushFrames();
    expect(editorHarness.onRestore).not.toHaveBeenCalled();

    view.rerender(
      <MonacoDiffViewer
        key="replacement-target"
        data={data("new target\n")}
        compact
        hideHeader
        sessionId="session-1"
      />,
    );
    flushFrames();
    expect(editorHarness.onRestore).not.toHaveBeenCalled();
  });
});
