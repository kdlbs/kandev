import { act, fireEvent, render } from "@testing-library/react";
import type { EditorView } from "@codemirror/view";
import { useLayoutEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const editorHarness = vi.hoisted(() => ({
  autoCreateView: null as EditorView | null,
  extensions: [] as unknown[],
  onCreateEditor: null as ((view: EditorView) => void) | null,
  onChange: undefined as ((content: string) => void) | undefined,
  readOnly: true,
}));
const revealPendingCodeMirrorCursor = vi.hoisted(() => vi.fn());
const clearCodeMirrorCursorFlash = vi.hoisted(() => vi.fn());
const codeMirrorCursorFlashExtension = vi.hoisted(() => ({}));

vi.mock("@uiw/react-codemirror", () => ({
  default: ({
    extensions,
    onCreateEditor,
    onChange,
    readOnly,
  }: {
    extensions: unknown[];
    onCreateEditor: (view: EditorView) => void;
    onChange?: (content: string) => void;
    readOnly?: boolean;
  }) => {
    editorHarness.extensions = extensions;
    editorHarness.onCreateEditor = onCreateEditor;
    editorHarness.onChange = onChange;
    editorHarness.readOnly = readOnly ?? false;
    useLayoutEffect(() => {
      if (editorHarness.autoCreateView) onCreateEditor(editorHarness.autoCreateView);
    }, [onCreateEditor]);
    return (
      <div>
        <div data-testid="file-viewer-codemirror" />
        <button type="button" onClick={() => onChange?.("edited")}>
          Change content
        </button>
      </div>
    );
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

afterEach(() => {
  editorHarness.autoCreateView = null;
  editorHarness.onChange = undefined;
  editorHarness.readOnly = true;
  vi.clearAllMocks();
});

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
    expect(revealPendingCodeMirrorCursor).toHaveBeenCalledTimes(1);

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

  it("consumes the initial pending reveal once when CodeMirror creates in a layout effect", () => {
    const view = {} as EditorView;
    editorHarness.autoCreateView = view;

    render(
      <FileViewerContent
        path="src/first.ts"
        content="first"
        repo="frontend"
        sessionId="session-1"
      />,
    );

    expect(revealPendingCodeMirrorCursor).toHaveBeenCalledTimes(1);
  });

  it("supports an editable CodeMirror source surface when requested", () => {
    const onChange = vi.fn();

    const rendered = render(
      <FileViewerContent path="README.md" content="# README" editable onChange={onChange} />,
    );

    expect(editorHarness.readOnly).toBe(false);
    expect(editorHarness.onChange).toBe(onChange);
    fireEvent.click(rendered.container.querySelector("button")!);
    expect(onChange).toHaveBeenCalledWith("edited");
  });
});
