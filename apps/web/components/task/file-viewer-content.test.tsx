import { act, render } from "@testing-library/react";
import type { EditorView } from "@codemirror/view";
import { describe, expect, it, vi } from "vitest";

const editorHarness = vi.hoisted(() => ({
  extensions: [] as unknown[],
  onCreateEditor: null as ((view: EditorView) => void) | null,
}));
const revealPendingCodeMirrorCursor = vi.hoisted(() => vi.fn());
const clearCodeMirrorCursorFlash = vi.hoisted(() => vi.fn());
const codeMirrorCursorFlashExtension = vi.hoisted(() => ({}));

vi.mock("@uiw/react-codemirror", () => ({
  default: ({
    extensions,
    onCreateEditor,
  }: {
    extensions: unknown[];
    onCreateEditor: (view: EditorView) => void;
  }) => {
    editorHarness.extensions = extensions;
    editorHarness.onCreateEditor = onCreateEditor;
    return <div data-testid="file-viewer-codemirror" />;
  },
}));

vi.mock("@/lib/languages", () => ({
  getCodeMirrorExtensionFromPath: () => null,
}));

vi.mock("@/components/editors/codemirror/use-codemirror-walkthrough-range", () => ({
  useCodeMirrorWalkthroughRange: () => null,
}));

vi.mock("@/components/editors/codemirror/codemirror-cursor-navigation", () => ({
  clearCodeMirrorCursorFlash,
  codeMirrorCursorFlashExtension,
  revealPendingCodeMirrorCursor,
}));

vi.mock("@/hooks/use-file-editors", () => ({
  consumePendingCursorPosition: () => undefined,
}));

import { FileViewerContent } from "./file-viewer-content";

describe("FileViewerContent cursor navigation", () => {
  it("uses shared CodeMirror reveal behavior on mount and reused path changes", () => {
    const view = {} as EditorView;
    const rendered = render(
      <FileViewerContent
        path="src/first.ts"
        content="first"
        repo="frontend"
        sessionId="session-1"
      />,
    );

    act(() => editorHarness.onCreateEditor?.(view));
    expect(editorHarness.extensions).toContain(codeMirrorCursorFlashExtension);
    expect(revealPendingCodeMirrorCursor).toHaveBeenLastCalledWith(
      view,
      "src/first.ts",
      "frontend",
      "session-1",
    );

    rendered.rerender(
      <FileViewerContent
        path="src/second.ts"
        content="second"
        repo="frontend"
        sessionId="session-1"
      />,
    );
    expect(clearCodeMirrorCursorFlash).toHaveBeenLastCalledWith(view);
    expect(revealPendingCodeMirrorCursor).toHaveBeenLastCalledWith(
      view,
      "src/second.ts",
      "frontend",
      "session-1",
    );
  });
});
