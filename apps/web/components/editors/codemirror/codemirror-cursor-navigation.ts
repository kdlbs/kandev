import type { Text } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { consumePendingCursorPosition } from "@/hooks/file-editor-cursor";

function clampOneBased(value: number, maximum: number): number {
  const integer = Number.isNaN(value) ? 1 : Math.trunc(value);
  return Math.min(Math.max(integer, 1), maximum);
}

export function getCodeMirrorCursorOffset(doc: Text, line: number, column: number): number {
  const targetLine = doc.line(clampOneBased(line, doc.lines));
  const targetColumn = clampOneBased(column, targetLine.length + 1);
  return targetLine.from + targetColumn - 1;
}

export function revealCodeMirrorCursor(view: EditorView, line: number, column: number): boolean {
  const anchor = getCodeMirrorCursorOffset(view.state.doc, line, column);
  view.dispatch({
    selection: { anchor },
    effects: EditorView.scrollIntoView(anchor, { y: "center" }),
  });
  view.focus();
  return true;
}

export function revealPendingCodeMirrorCursor(
  view: EditorView,
  path: string,
  repo?: string,
): boolean {
  const pending = consumePendingCursorPosition(path, repo);
  if (!pending) return false;
  return revealCodeMirrorCursor(view, pending.line, pending.column);
}
