import type { EditorState } from "@codemirror/state";
import type { EditorView } from "@codemirror/view";

export type CodeMirrorSelection = { text: string; startLine: number; endLine: number };

/** Main selection with its 1-based line range, or null when empty or whitespace-only. */
export function readCodeMirrorSelection(state: EditorState): CodeMirrorSelection | null {
  const selection = state.selection.main;
  if (selection.empty) return null;
  const text = state.sliceDoc(selection.from, selection.to);
  if (!text.trim()) return null;
  return {
    text,
    startLine: state.doc.lineAt(selection.from).number,
    endLine: state.doc.lineAt(selection.to).number,
  };
}

/** Viewport point just below the end of the main selection. */
export function selectionEndPoint(view: EditorView): { x: number; y: number } {
  try {
    const coords = view.coordsAtPos(view.state.selection.main.to);
    if (coords) return { x: coords.left, y: coords.bottom };
  } catch {
    // Layout reads can fail before the editor is measured.
  }
  const rect = view.dom.getBoundingClientRect();
  return { x: rect.left, y: rect.top };
}
