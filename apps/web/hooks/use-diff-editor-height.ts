import { useState, useEffect, useMemo } from 'react';
import type { editor as monacoEditor } from 'monaco-editor';

interface UseDiffEditorHeightOptions {
  modifiedEditor: monacoEditor.ICodeEditor | null;
  originalEditor: monacoEditor.ICodeEditor | null;
  compact: boolean;
  lineHeight: number;
  originalContent: string;
  modifiedContent: string;
}

const MIN_HEIGHT = 80;
const MAX_HEIGHT_NORMAL = 600;
const MAX_HEIGHT_COMPACT = 300;

function clampHeight(h: number, compact: boolean): number {
  const max = compact ? MAX_HEIGHT_COMPACT : MAX_HEIGHT_NORMAL;
  return Math.min(Math.max(h, MIN_HEIGHT), max);
}

export function useDiffEditorHeight({
  modifiedEditor,
  originalEditor,
  compact,
  lineHeight,
  originalContent,
  modifiedContent,
}: UseDiffEditorHeightOptions): number {
  // Estimate before editors mount
  const estimatedHeight = useMemo(() => {
    const lines = Math.max(
      originalContent.split('\n').length,
      modifiedContent.split('\n').length,
    );
    return clampHeight(lines * lineHeight + 10, compact);
  }, [originalContent, modifiedContent, lineHeight, compact]);

  const [editorHeight, setEditorHeight] = useState<number | null>(null);

  // Subscribe to content size changes on both editors
  useEffect(() => {
    if (!modifiedEditor || !originalEditor) return;

    const update = () => {
      const modH = modifiedEditor.getContentHeight();
      const origH = originalEditor.getContentHeight();
      setEditorHeight(clampHeight(Math.max(modH, origH), compact));
    };

    update();

    const d1 = modifiedEditor.onDidContentSizeChange(update);
    const d2 = originalEditor.onDidContentSizeChange(update);

    return () => {
      d1.dispose();
      d2.dispose();
      setEditorHeight(null);
    };
  }, [modifiedEditor, originalEditor, compact]);

  return editorHeight ?? estimatedHeight;
}
