"use client";

import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { DiffEditor } from "@monaco-editor/react";
import { useTheme } from "next-themes";
import { cn } from "@kandev/ui/lib/utils";
import type { FileDiffData, DiffComment } from "@/lib/diff/types";
import { getMonacoLanguage } from "@/lib/editor/language-map";
import { useCommandPanelOpen } from "@/lib/commands/command-registry";
import { useDiffEditorHeight } from "@/hooks/use-diff-editor-height";
import { useGlobalViewMode } from "@/hooks/use-global-view-mode";
import { useCaptureKeydown } from "@/hooks/use-capture-keydown";
import { DiffViewerToolbar } from "./diff-viewer-toolbar";
import { DiffViewerContextMenu, type ContextMenuState } from "./diff-viewer-context-menu";
import { useDiffViewerComments } from "./use-diff-viewer-comments";
import { useGlobalFolding } from "./use-global-folding";
import { resolveDiffContent, buildDiffEditorOptions } from "./diff-viewer-helpers";
import { initMonacoThemes } from "./monaco-init";

initMonacoThemes();

function getMonacoTheme(resolvedTheme: string | undefined): string {
  return resolvedTheme === "dark" ? "kandev-dark" : "kandev-light";
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

function useMonacoDiffViewerState(props: MonacoDiffViewerProps) {
  const { data, sessionId, compact = false, onCommentAdd, onCommentDelete, comments: externalComments, onModifiedContentChange, wordWrap: wordWrapProp, editable, onRevert } = props;
  const { resolvedTheme } = useTheme();
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const [foldUnchanged, setFoldUnchanged] = useGlobalFolding();
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();

  const {
    diffEditorRef,
    modifiedEditor,
    originalEditor,
    contextMenu,
    setContextMenu,
    copyAllChangedLines,
    handleDiffEditorMount,
  } = useDiffViewerComments({ data, sessionId, compact, onCommentAdd, onCommentDelete, externalComments, onModifiedContentChange });

  useCaptureKeydown(wrapperRef, { metaOrCtrl: true, key: "k" }, () => setCommandPanelOpen(true));

  useLayoutEffect(() => {
    const ref = diffEditorRef;
    return () => {
      try { ref.current?.setModel(null); } catch { /* already disposed */ }
    };
  }, [diffEditorRef]);

  const { oldContent, newContent, diff, filePath } = data;
  const language = getMonacoLanguage(filePath);
  const { original, modified } = useMemo(
    () => resolveDiffContent({ oldContent, newContent, diff }),
    [oldContent, newContent, diff],
  );
  const lineHeight = compact ? 16 : 18;
  const editorHeight = useDiffEditorHeight({ modifiedEditor, originalEditor, compact, lineHeight, originalContent: original, modifiedContent: modified });
  const modifiedReadOnly = compact || (!editable && !onRevert);
  const monacoTheme = getMonacoTheme(resolvedTheme);
  const hasDiff = !!(oldContent || newContent || diff);

  const options = buildDiffEditorOptions({ compact, wordWrap, modifiedReadOnly, onRevert, globalViewMode, foldUnchanged, lineHeight });

  return { resolvedTheme, globalViewMode, setGlobalViewMode, foldUnchanged, setFoldUnchanged, wordWrap, setWordWrap, wrapperRef, contextMenu, setContextMenu, copyAllChangedLines, handleDiffEditorMount, diff, filePath, language, original, modified, lineHeight, editorHeight, hasDiff, monacoTheme, options };
}

export function MonacoDiffViewer(props: MonacoDiffViewerProps) {
  const { className, compact = false, hideHeader = false, onOpenFile, onRevert } = props;
  const state = useMonacoDiffViewerState(props);
  const { wrapperRef, hasDiff, contextMenu, setContextMenu } = state;
  const showHeader = !hideHeader && !compact;

  if (!hasDiff) {
    return (
      <div
        className={cn(
          "rounded-md border border-border/50 bg-muted/20 p-4 text-muted-foreground",
          compact ? "text-xs" : "text-sm",
          className,
        )}
      >
        No diff available
      </div>
    );
  }

  return (
    <div ref={wrapperRef} className={cn("monaco-diff-viewer relative", className)}>
      {showHeader && (
        <DiffViewerToolbar
          data={props.data}
          foldUnchanged={state.foldUnchanged}
          setFoldUnchanged={state.setFoldUnchanged}
          wordWrap={state.wordWrap}
          setWordWrap={state.setWordWrap}
          globalViewMode={state.globalViewMode}
          setGlobalViewMode={state.setGlobalViewMode}
          onCopyDiff={() => navigator.clipboard.writeText(state.diff ?? "")}
          onOpenFile={onOpenFile}
          onRevert={onRevert}
        />
      )}
      <div
        className={cn(
          "overflow-hidden",
          showHeader ? "rounded-b-md" : "rounded-md",
          "border border-border/50",
        )}
      >
        <DiffEditor
          height={state.editorHeight}
          language={state.language}
          original={state.original}
          modified={state.modified}
          theme={state.monacoTheme}
          onMount={state.handleDiffEditorMount}
          options={state.options}
          loading={
            <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
              Loading diff...
            </div>
          }
        />
      </div>
      {contextMenu && (
        <DiffViewerContextMenu
          contextMenu={contextMenu as NonNullable<ContextMenuState>}
          onCopyAllChanged={state.copyAllChangedLines}
          onClose={() => setContextMenu(null)}
          onRevert={onRevert}
          filePath={state.filePath}
        />
      )}
    </div>
  );
}
