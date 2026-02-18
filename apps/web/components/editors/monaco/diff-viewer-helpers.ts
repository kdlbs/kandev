import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE } from '@/lib/theme/editor-theme';

/** Parse original/modified sides from a unified diff patch. */
export function parseDiffContent(diff: string): { original: string; modified: string } {
  const originalLines: string[] = [];
  const modifiedLines: string[] = [];
  for (const line of diff.split('\n')) {
    if (line.startsWith('diff ') || line.startsWith('index ') ||
        line.startsWith('--- ') || line.startsWith('+++ ') ||
        line.startsWith('@@') || line.startsWith('\\')) continue;
    if (line.startsWith('-')) {
      originalLines.push(line.slice(1));
    } else if (line.startsWith('+')) {
      modifiedLines.push(line.slice(1));
    } else {
      const content = line.startsWith(' ') ? line.slice(1) : line;
      originalLines.push(content);
      modifiedLines.push(content);
    }
  }
  return { original: originalLines.join('\n'), modified: modifiedLines.join('\n') };
}

/** Resolve original/modified content from diff fields. */
export function resolveDiffContent(fields: {
  oldContent?: string;
  newContent?: string;
  diff?: string;
}): { original: string; modified: string } {
  if (fields.oldContent || fields.newContent) {
    return { original: fields.oldContent ?? '', modified: fields.newContent ?? '' };
  }
  if (fields.diff) return parseDiffContent(fields.diff);
  return { original: '', modified: '' };
}

type DiffEditorOptionsArgs = {
  compact: boolean;
  wordWrap: boolean;
  modifiedReadOnly: boolean;
  onRevert?: ((filePath: string) => void) | undefined;
  globalViewMode: string;
  foldUnchanged: boolean;
  lineHeight: number;
};

/** Build the Monaco DiffEditor options object. */
export function buildDiffEditorOptions(args: DiffEditorOptionsArgs) {
  const { compact, wordWrap, modifiedReadOnly, onRevert, globalViewMode, foldUnchanged, lineHeight } = args;
  return {
    fontSize: compact ? 11 : EDITOR_FONT_SIZE,
    fontFamily: EDITOR_FONT_FAMILY,
    lineHeight,
    minimap: { enabled: false },
    wordWrap: wordWrap ? 'on' as const : 'off' as const,
    readOnly: modifiedReadOnly,
    originalEditable: false,
    renderMarginRevertIcon: !compact && !!onRevert,
    contextmenu: false,
    renderSideBySide: globalViewMode === 'split',
    scrollBeyondLastLine: false,
    smoothScrolling: true,
    automaticLayout: true,
    renderOverviewRuler: false,
    hideUnchangedRegions: {
      enabled: foldUnchanged,
      contextLineCount: 3,
      minimumLineCount: 3,
      revealLineCount: 20,
    },
    folding: !compact,
    lineNumbers: compact ? 'off' as const : 'on' as const,
    glyphMargin: false,
    lineDecorationsWidth: 10,
    scrollbar: {
      verticalScrollbarSize: 8,
      horizontalScrollbarSize: 8,
      alwaysConsumeMouseWheel: false,
    },
    padding: { top: 2 },
  };
}
