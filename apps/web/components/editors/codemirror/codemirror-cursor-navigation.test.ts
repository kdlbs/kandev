import { EditorState, Text } from "@codemirror/state";
import { EditorView as CodeMirrorView, type EditorView } from "@codemirror/view";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS,
  EDITOR_CONTENT_SEARCH_FLASH_CLASS,
} from "@/hooks/file-editor-cursor";
import {
  codeMirrorCursorFlashExtension,
  getCodeMirrorCursorOffset,
  revealCodeMirrorCursor,
} from "./codemirror-cursor-navigation";

const DOCUMENT = Text.of(["alpha", "bravo", "charlie"]);

afterEach(() => {
  document.body.replaceChildren();
  vi.useRealTimers();
});

describe("getCodeMirrorCursorOffset", () => {
  it("translates a 1-based line and column into a document offset", () => {
    expect(getCodeMirrorCursorOffset(DOCUMENT, 2, 3)).toBe(8);
  });

  it("clamps a position before the document to its start", () => {
    expect(getCodeMirrorCursorOffset(DOCUMENT, -4, 0)).toBe(0);
  });

  it("clamps a position after the document to its end", () => {
    expect(getCodeMirrorCursorOffset(DOCUMENT, 99, 99)).toBe(DOCUMENT.length);
  });
});

describe("revealCodeMirrorCursor", () => {
  it("dispatches a line-flash effect for a flashed reveal", () => {
    const dispatch = vi.fn();
    const view = {
      state: EditorState.create({ doc: DOCUMENT }),
      dispatch,
      focus: vi.fn(),
    } as unknown as EditorView;

    revealCodeMirrorCursor(view, 2, 3, { flashLine: true });

    const request = dispatch.mock.calls[0][0] as { effects: unknown };
    expect(Array.isArray(request.effects)).toBe(true);
  });

  it("renders the destination line flash for a bounded interval", () => {
    vi.useFakeTimers();
    const parent = document.createElement("div");
    document.body.append(parent);
    const view = new CodeMirrorView({
      parent,
      state: EditorState.create({
        doc: DOCUMENT,
        extensions: [codeMirrorCursorFlashExtension],
      }),
    });

    revealCodeMirrorCursor(view, 2, 3, { flashLine: true });
    expect(parent.querySelector(`.${EDITOR_CONTENT_SEARCH_FLASH_CLASS}`)?.textContent).toBe(
      "bravo",
    );

    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS);
    expect(parent.querySelector(`.${EDITOR_CONTENT_SEARCH_FLASH_CLASS}`)).toBeNull();
    view.destroy();
  });

  it("restarts the flash without letting the older timer clear it", () => {
    vi.useFakeTimers();
    const parent = document.createElement("div");
    document.body.append(parent);
    const view = new CodeMirrorView({
      parent,
      state: EditorState.create({
        doc: DOCUMENT,
        extensions: [codeMirrorCursorFlashExtension],
      }),
    });

    revealCodeMirrorCursor(view, 1, 1, { flashLine: true });
    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS / 2);
    revealCodeMirrorCursor(view, 3, 1, { flashLine: true });
    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS / 2);
    expect(parent.querySelector(`.${EDITOR_CONTENT_SEARCH_FLASH_CLASS}`)?.textContent).toBe(
      "charlie",
    );

    vi.advanceTimersByTime(EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS / 2);
    expect(parent.querySelector(`.${EDITOR_CONTENT_SEARCH_FLASH_CLASS}`)).toBeNull();
    view.destroy();
  });
});
