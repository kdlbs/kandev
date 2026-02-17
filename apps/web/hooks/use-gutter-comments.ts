import { useEffect, useRef, useCallback, useState } from 'react';
import type { editor as monacoEditor } from 'monaco-editor';

type SelectionCompleteParams = {
  range: { start: number; end: number };
  code: string;
  position: { x: number; y: number };
};

type UseGutterCommentsOptions = {
  enabled: boolean;
  /** Lines that already have comments — skip hover "+" for these */
  commentedLines: Set<number>;
  onSelectionComplete: (params: SelectionCompleteParams) => void;
};

const LINE_NUMBERS = 3;
const LINE_DECORATIONS = 4;
function isGutterTarget(type: number) {
  return type === LINE_NUMBERS || type === LINE_DECORATIONS;
}

/** Apply selection decorations for a line range (no linesDecorations — avoids overlap with comment bar) */
function applySelectionDecos(
  collection: monacoEditor.IEditorDecorationsCollection | null,
  start: number,
  end: number,
) {
  if (!collection) return;
  const decos: monacoEditor.IModelDeltaDecoration[] = [];
  for (let l = start; l <= end; l++) {
    decos.push({
      range: { startLineNumber: l, startColumn: 1, endLineNumber: l, endColumn: 1 },
      options: {
        isWholeLine: true,
        className: 'monaco-gutter-selected-line',
        lineNumberClassName: 'monaco-gutter-selected-line',
        linesDecorationsClassName: 'monaco-gutter-selected-deco',
      },
    });
  }
  collection.set(decos);
}

/** Show "+" hover hint on the lines-decoration lane (same lane as comment icons) */
function showHoverHint(
  collection: monacoEditor.IEditorDecorationsCollection | null,
  line: number,
) {
  collection?.set([{
    range: { startLineNumber: line, startColumn: 1, endLineNumber: line, endColumn: 1 },
    options: { linesDecorationsClassName: 'monaco-gutter-comment-hint' },
  }]);
}

/** Get viewport-relative position for the bottom of a line in the editor */
export function getLineBottomPosition(
  editor: monacoEditor.ICodeEditor,
  lineNumber: number,
): { x: number; y: number } | null {
  const pos = editor.getScrolledVisiblePosition({ lineNumber, column: 1 });
  const dom = editor.getDomNode();
  if (!pos || !dom) return null;
  const rect = dom.getBoundingClientRect();
  return { x: rect.left + pos.left, y: rect.top + pos.top + pos.height };
}

/**
 * GitHub PR-style gutter interactions for Monaco:
 * - Hover "+" icon on line numbers (no glyph margin needed)
 * - Click-and-drag to select line range, shift-click to extend
 * - Calls onSelectionComplete with the selected code and position
 */
export function useGutterComments(
  editor: monacoEditor.ICodeEditor | null,
  options: UseGutterCommentsOptions,
) {
  const { enabled, commentedLines, onSelectionComplete } = options;

  const [gutterSelection, setGutterSelection] = useState<{ startLine: number; endLine: number } | null>(null);
  const anchorLineRef = useRef<number | null>(null);
  const isDraggingRef = useRef(false);
  const hasSelectionRef = useRef(false);
  const hoverDecRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const selectionDecRef = useRef<monacoEditor.IEditorDecorationsCollection | null>(null);
  const callbackRef = useRef(onSelectionComplete);
  const commentedRef = useRef(commentedLines);

  useEffect(() => { callbackRef.current = onSelectionComplete; }, [onSelectionComplete]);
  useEffect(() => { commentedRef.current = commentedLines; }, [commentedLines]);
  useEffect(() => { hasSelectionRef.current = gutterSelection !== null; }, [gutterSelection]);

  const clearGutterSelection = useCallback(() => {
    setGutterSelection(null);
    anchorLineRef.current = null;
    selectionDecRef.current?.set([]);
  }, []);

  // Hover "+" icon — shown on gutter mousemove
  useEffect(() => {
    if (!editor || !enabled) return;
    hoverDecRef.current = editor.createDecorationsCollection([]);
    const moveSub = editor.onMouseMove((e) => {
      const line = e.target.position?.lineNumber;
      if (!isGutterTarget(e.target.type) || !line || commentedRef.current.has(line) || hasSelectionRef.current) {
        hoverDecRef.current?.set([]);
        return;
      }
      showHoverHint(hoverDecRef.current, line);
    });
    const leaveSub = editor.onMouseLeave(() => hoverDecRef.current?.set([]));
    return () => { moveSub.dispose(); leaveSub.dispose(); hoverDecRef.current?.set([]); };
  }, [editor, enabled]);

  // Drag-to-select: mousedown → DOM mousemove → DOM mouseup
  useEffect(() => {
    if (!editor || !enabled) return;
    selectionDecRef.current = editor.createDecorationsCollection([]);

    let dragStart = 0;
    let dragEnd = 0;

    const finishDrag = () => {
      if (!isDraggingRef.current) return;
      isDraggingRef.current = false;
      document.removeEventListener('mousemove', onDomMouseMove);
      document.removeEventListener('mouseup', onDomMouseUp);

      const model = editor.getModel();
      if (!model) return;
      const s = Math.min(dragStart, dragEnd);
      const e = Math.max(dragStart, dragEnd);
      const code = model.getValueInRange({
        startLineNumber: s, startColumn: 1,
        endLineNumber: e, endColumn: model.getLineMaxColumn(e),
      });
      setGutterSelection({ startLine: s, endLine: e });
      // Position popover right below the last selected line
      const pos = getLineBottomPosition(editor, e) ?? { x: 0, y: 0 };
      callbackRef.current({ range: { start: s, end: e }, code, position: pos });
    };

    const onDomMouseMove = (ev: MouseEvent) => {
      if (!isDraggingRef.current) return;
      const target = editor.getTargetAtClientPoint(ev.clientX, ev.clientY);
      const line = target?.position?.lineNumber;
      if (!line) return;
      dragEnd = line;
      const s = Math.min(dragStart, dragEnd);
      const e = Math.max(dragStart, dragEnd);
      applySelectionDecos(selectionDecRef.current, s, e);
      setGutterSelection({ startLine: s, endLine: e });
      // Clear hover hint during drag — selection decos provide visual feedback
      hoverDecRef.current?.set([]);
    };

    const onDomMouseUp = () => finishDrag();

    const mouseDownSub = editor.onMouseDown((e) => {
      const line = e.target.position?.lineNumber;
      if (!isGutterTarget(e.target.type) || !line) return;
      if (commentedRef.current.has(line)) return;

      // Shift-click extends from existing anchor
      if (e.event.shiftKey && anchorLineRef.current !== null) {
        dragStart = Math.min(anchorLineRef.current, line);
        dragEnd = Math.max(anchorLineRef.current, line);
        applySelectionDecos(selectionDecRef.current, dragStart, dragEnd);
        finishDrag();
        return;
      }

      isDraggingRef.current = true;
      anchorLineRef.current = line;
      dragStart = line;
      dragEnd = line;
      applySelectionDecos(selectionDecRef.current, line, line);
      document.addEventListener('mousemove', onDomMouseMove);
      document.addEventListener('mouseup', onDomMouseUp);
    });

    return () => {
      mouseDownSub.dispose();
      document.removeEventListener('mousemove', onDomMouseMove);
      document.removeEventListener('mouseup', onDomMouseUp);
      isDraggingRef.current = false;
      selectionDecRef.current?.set([]);
    };
  }, [editor, enabled]);

  return { gutterSelection, clearGutterSelection };
}
