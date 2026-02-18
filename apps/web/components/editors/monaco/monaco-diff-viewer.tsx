'use client';

import { useLayoutEffect, useMemo, useRef, useState } from 'react';
import { DiffEditor } from '@monaco-editor/react';
import { useTheme } from 'next-themes';
import { cn } from '@kandev/ui/lib/utils';
import type { FileDiffData, DiffComment } from '@/lib/diff/types';
import { getMonacoLanguage } from '@/lib/editor/language-map';
import { useCommandPanelOpen } from '@/lib/commands/command-registry';
import { useDiffEditorHeight } from '@/hooks/use-diff-editor-height';
import { useGlobalViewMode } from '@/hooks/use-global-view-mode';
import { useCaptureKeydown } from '@/hooks/use-capture-keydown';
import { DiffViewerToolbar } from './diff-viewer-toolbar';
import { DiffViewerContextMenu } from './diff-viewer-context-menu';
import { useDiffViewerComments } from './use-diff-viewer-comments';
import { useGlobalFolding } from './use-global-folding';
import { resolveDiffContent, buildDiffEditorOptions } from './diff-viewer-helpers';
import { initMonacoThemes } from './monaco-init';

initMonacoThemes();

function getMonacoTheme(resolvedTheme: string | undefined): string {
  return resolvedTheme === 'dark' ? 'kandev-dark' : 'kandev-light';
}

interface MonacoDiffViewerProps {
  data: FileDiffData;
  sessionId?: string;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  comments?: DiffComment[];
  className?: string;
  compact?: boolean;
  hideHeader?: boolean;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  wordWrap?: boolean;
  editable?: boolean;
  onModifiedContentChange?: (filePath: string, content: string) => void;
}

export function MonacoDiffViewer({
  data, sessionId, onCommentAdd, onCommentDelete,
  comments: externalComments, className, compact = false,
  hideHeader = false, onOpenFile, onRevert, wordWrap: wordWrapProp,
  editable, onModifiedContentChange,
}: MonacoDiffViewerProps) {
  const { resolvedTheme } = useTheme();
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const [foldUnchanged, setFoldUnchanged] = useGlobalFolding();
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();

  const {
    diffEditorRef, modifiedEditor, originalEditor,
    contextMenu, setContextMenu, copyAllChangedLines,
    handleDiffEditorMount,
  } = useDiffViewerComments({
    data, sessionId, compact, onCommentAdd, onCommentDelete,
    externalComments, onModifiedContentChange,
  });

  // Cmd+K for command panel (capture phase to intercept before Monaco)
  useCaptureKeydown(wrapperRef, { metaOrCtrl: true, key: 'k' }, () => setCommandPanelOpen(true));

  // Fix: reset the DiffEditor model BEFORE cleanup
  useLayoutEffect(() => {
    const ref = diffEditorRef;
    return () => {
      try { ref.current?.setModel(null); } catch { /* already disposed */ }
    };
  }, [diffEditorRef]);

  const { oldContent, newContent, diff, filePath } = data;
  const language = getMonacoLanguage(filePath);
  const showHeader = !hideHeader && !compact;
  const { original, modified } = useMemo(
    () => resolveDiffContent({ oldContent, newContent, diff }),
    [oldContent, newContent, diff],
  );
  const lineHeight = compact ? 16 : 18;
  const editorHeight = useDiffEditorHeight({
    modifiedEditor, originalEditor, compact, lineHeight,
    originalContent: original, modifiedContent: modified,
  });
  const hasDiff = !!(oldContent || newContent || diff);

  if (!hasDiff) {
    return (
      <div className={cn('rounded-md border border-border/50 bg-muted/20 p-4 text-muted-foreground', compact ? 'text-xs' : 'text-sm', className)}>
        No diff available
      </div>
    );
  }

  const monacoTheme = getMonacoTheme(resolvedTheme);
  const modifiedReadOnly = compact || (!editable && !onRevert);
  const options = buildDiffEditorOptions({ compact, wordWrap, modifiedReadOnly, onRevert, globalViewMode, foldUnchanged, lineHeight });

  return (
    <div ref={wrapperRef} className={cn('monaco-diff-viewer relative', className)}>
      {showHeader && (
        <DiffViewerToolbar
          data={data} foldUnchanged={foldUnchanged} setFoldUnchanged={setFoldUnchanged}
          wordWrap={wordWrap} setWordWrap={setWordWrap}
          globalViewMode={globalViewMode} setGlobalViewMode={setGlobalViewMode}
          onCopyDiff={() => navigator.clipboard.writeText(diff ?? '')}
          onOpenFile={onOpenFile} onRevert={onRevert}
        />
      )}
      <div className={cn('overflow-hidden', showHeader ? 'rounded-b-md' : 'rounded-md', 'border border-border/50')}>
        <DiffEditor
          height={editorHeight} language={language} original={original} modified={modified}
          theme={monacoTheme} onMount={handleDiffEditorMount} options={options}
          loading={<div className="flex h-full items-center justify-center text-muted-foreground text-sm">Loading diff...</div>}
        />
      </div>
      {contextMenu && (
        <DiffViewerContextMenu contextMenu={contextMenu} onCopyAllChanged={copyAllChangedLines}
          onClose={() => setContextMenu(null)} onRevert={onRevert} filePath={filePath} />
      )}
    </div>
  );
}
