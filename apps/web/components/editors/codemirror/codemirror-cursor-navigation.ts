import { StateEffect, StateField, type Extension, type Text } from "@codemirror/state";
import { Decoration, EditorView, type DecorationSet } from "@codemirror/view";
import {
  consumePendingCursorPosition,
  EDITOR_CONTENT_SEARCH_FLASH_CLASS,
  EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS,
  type CursorRevealOptions,
} from "@/hooks/file-editor-cursor";

const setCursorLineFlash = StateEffect.define<number | null>();
const cursorLineFlashField = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update: (decorations, transaction) => {
    let next = decorations.map(transaction.changes);
    for (const effect of transaction.effects) {
      if (!effect.is(setCursorLineFlash)) continue;
      if (effect.value === null) return Decoration.none;
      const line = transaction.state.doc.line(
        clampOneBased(effect.value, transaction.state.doc.lines),
      );
      next = Decoration.set([
        Decoration.line({ class: EDITOR_CONTENT_SEARCH_FLASH_CLASS }).range(line.from),
      ]);
    }
    return next;
  },
  provide: (field) => EditorView.decorations.from(field),
});

export const codeMirrorCursorFlashExtension: Extension = cursorLineFlashField;

type CodeMirrorCursorFlash = { timeout: ReturnType<typeof setTimeout> };
const codeMirrorCursorFlashes = new WeakMap<EditorView, CodeMirrorCursorFlash>();

function clampOneBased(value: number, maximum: number): number {
  const integer = Number.isNaN(value) ? 1 : Math.trunc(value);
  return Math.min(Math.max(integer, 1), maximum);
}

export function getCodeMirrorCursorOffset(doc: Text, line: number, column: number): number {
  const targetLine = doc.line(clampOneBased(line, doc.lines));
  const targetColumn = clampOneBased(column, targetLine.length + 1);
  return targetLine.from + targetColumn - 1;
}

function scheduleCodeMirrorCursorFlashCleanup(view: EditorView): void {
  const previous = codeMirrorCursorFlashes.get(view);
  if (previous) clearTimeout(previous.timeout);
  const flash: CodeMirrorCursorFlash = {
    timeout: setTimeout(() => {
      if (codeMirrorCursorFlashes.get(view) !== flash) return;
      view.dispatch({ effects: setCursorLineFlash.of(null) });
      codeMirrorCursorFlashes.delete(view);
    }, EDITOR_CONTENT_SEARCH_FLASH_DURATION_MS),
  };
  codeMirrorCursorFlashes.set(view, flash);
}

export function clearCodeMirrorCursorFlash(view: EditorView): void {
  const flash = codeMirrorCursorFlashes.get(view);
  if (!flash) return;
  clearTimeout(flash.timeout);
  view.dispatch({ effects: setCursorLineFlash.of(null) });
  codeMirrorCursorFlashes.delete(view);
}

export function revealCodeMirrorCursor(
  view: EditorView,
  line: number,
  column: number,
  options: CursorRevealOptions = {},
): boolean {
  const anchor = getCodeMirrorCursorOffset(view.state.doc, line, column);
  const effects = [EditorView.scrollIntoView(anchor, { y: "center" })];
  if (options.flashLine) effects.push(setCursorLineFlash.of(line));
  view.dispatch({
    selection: { anchor },
    effects,
  });
  if (options.flashLine) scheduleCodeMirrorCursorFlashCleanup(view);
  view.focus();
  return true;
}

export function revealPendingCodeMirrorCursor(
  view: EditorView,
  path: string,
  repo?: string,
  sessionId?: string,
): boolean {
  const pending = consumePendingCursorPosition(path, repo, sessionId);
  if (!pending) return false;
  return revealCodeMirrorCursor(view, pending.line, pending.column, {
    flashLine: pending.flashLine,
  });
}
