"use client";

import {
  useCallback,
  useEffect,
  useInsertionEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { DiffEditor } from "@monaco-editor/react";
import { useTheme } from "@/components/theme/app-theme";
import { cn } from "@kandev/ui/lib/utils";
import type { FileDiffData, DiffComment, DiffCommentUpdate } from "@/lib/diff/types";
import { getMonacoLanguage } from "@/lib/editor/language-map";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
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
import { useTranslation } from "react-i18next";
import { djb2Hash } from "@/lib/utils/hash";
import type { editor as monacoEditor } from "monaco-editor";
import {
  captureContentLineAnchor,
  resolveContentLineAnchor,
  type ContentLine,
  type ContentLineAnchor,
} from "@/components/review/line-content-anchor";

initMonacoThemes();

function getMonacoTheme(resolvedTheme: string | undefined): string {
  return resolvedTheme === "dark" ? "kandev-dark" : "kandev-light";
}

type MonacoSelectionAnchor = {
  selectionStart: ContentLineAnchor | null;
  selectionStartColumn: number;
  position: ContentLineAnchor | null;
  positionColumn: number;
};

type MonacoPaneAnchor = {
  visibleLine: ContentLineAnchor | null;
  visibleOffset: number;
  selection: MonacoSelectionAnchor | null;
};

type PendingMonacoRestore = {
  viewState: monacoEditor.IDiffEditorViewState;
  modified: MonacoPaneAnchor | null;
  original: MonacoPaneAnchor | null;
};

function modelLines(content: string): ContentLine[] {
  return content.split(/\r?\n/).map((line, index) => ({ line: index + 1, content: line }));
}

function capturePaneAnchor(
  editor: monacoEditor.IStandaloneCodeEditor | null,
  content: string,
): MonacoPaneAnchor | null {
  if (!editor) return null;
  const lines = modelLines(content);
  const visibleLine = editor.getVisibleRanges()[0]?.startLineNumber ?? 1;
  const visibleOffset = editor.getTopForPosition(visibleLine, 1) - editor.getScrollTop();
  const selection = editor.getSelection();
  return {
    visibleLine: captureContentLineAnchor(lines, visibleLine - 1),
    visibleOffset,
    selection: selection
      ? {
          selectionStart: captureContentLineAnchor(lines, selection.selectionStartLineNumber - 1),
          selectionStartColumn: selection.selectionStartColumn,
          position: captureContentLineAnchor(lines, selection.positionLineNumber - 1),
          positionColumn: selection.positionColumn,
        }
      : null,
  };
}

function mappedLine(content: string, anchor: ContentLineAnchor): number {
  const lines = modelLines(content);
  const match = resolveContentLineAnchor(lines, anchor);
  return match?.line ?? Math.min(Math.max(1, anchor.line), Math.max(1, lines.length));
}

function restorePaneAnchor(
  editor: monacoEditor.IStandaloneCodeEditor | null,
  snapshot: MonacoPaneAnchor | null,
  content: string,
  restoreScroll = true,
) {
  if (!editor || !snapshot) return;
  if (restoreScroll && snapshot.visibleLine) {
    const line = mappedLine(content, snapshot.visibleLine);
    editor.setScrollTop(Math.max(0, editor.getTopForPosition(line, 1) - snapshot.visibleOffset));
  }

  const selection = snapshot.selection;
  if (!selection?.selectionStart || !selection.position) return;
  const startLine = mappedLine(content, selection.selectionStart);
  const positionLine = mappedLine(content, selection.position);
  const model = editor.getModel();
  const startColumn = Math.min(
    selection.selectionStartColumn,
    model?.getLineMaxColumn(startLine) ?? 1,
  );
  const positionColumn = Math.min(
    selection.positionColumn,
    model?.getLineMaxColumn(positionLine) ?? 1,
  );
  editor.setSelection({
    selectionStartLineNumber: startLine,
    selectionStartColumn: startColumn,
    positionLineNumber: positionLine,
    positionColumn,
  });
}

function useMonacoDiffViewState(
  modelKey: string,
  diffEditorRef: React.RefObject<monacoEditor.IStandaloneDiffEditor | null>,
  ready: boolean,
  originalContent: string,
  modifiedContent: string,
) {
  const previousModelKeyRef = useRef(modelKey);
  const previousContentsRef = useRef({ original: originalContent, modified: modifiedContent });
  const pendingRestoreRef = useRef<PendingMonacoRestore | null>(null);
  const restoreFrameRef = useRef<number | null>(null);

  useInsertionEffect(() => {
    if (previousModelKeyRef.current !== modelKey) {
      const editor = diffEditorRef.current;
      const previousContents = previousContentsRef.current;
      const viewState = editor?.saveViewState();
      pendingRestoreRef.current =
        editor && viewState
          ? {
              viewState,
              modified: capturePaneAnchor(editor.getModifiedEditor(), previousContents.modified),
              original: capturePaneAnchor(editor.getOriginalEditor(), previousContents.original),
            }
          : null;
      previousModelKeyRef.current = modelKey;
    }
  }, [diffEditorRef, modelKey]);

  useLayoutEffect(() => {
    previousContentsRef.current = { original: originalContent, modified: modifiedContent };
  }, [modifiedContent, originalContent]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    const editor = diffEditorRef.current;
    if (!ready || !editor || !pendingRestore) return;
    const scheduleRestore = () => {
      if (restoreFrameRef.current !== null) return;
      restoreFrameRef.current = requestAnimationFrame(() => {
        restoreFrameRef.current = null;
        if (pendingRestoreRef.current !== pendingRestore || diffEditorRef.current !== editor)
          return;
        const modifiedEditor = editor.getModifiedEditor();
        const originalEditor = editor.getOriginalEditor();
        const modifiedMatches = modifiedEditor.getValue() === modifiedContent;
        const originalMatches = !originalEditor || originalEditor.getValue() === originalContent;
        if (!modifiedMatches || !originalMatches) {
          return;
        }
        editor.restoreViewState(pendingRestore.viewState);
        modifiedEditor.layout();
        const modifiedLineCount = modifiedEditor.getModel()?.getLineCount() ?? 0;
        const originalLineCount = originalEditor?.getModel()?.getLineCount() ?? 0;
        const restoreModifiedScroll =
          !!pendingRestore.modified?.visibleLine &&
          (!pendingRestore.original?.visibleLine || modifiedLineCount >= originalLineCount);
        restorePaneAnchor(
          modifiedEditor,
          pendingRestore.modified,
          modifiedContent,
          restoreModifiedScroll,
        );
        restorePaneAnchor(
          originalEditor,
          pendingRestore.original,
          originalContent,
          !restoreModifiedScroll,
        );
        pendingRestoreRef.current = null;
      });
    };
    const modifiedEditor = editor.getModifiedEditor();
    const originalEditor = editor.getOriginalEditor();
    const modifiedContentListener = modifiedEditor.onDidChangeModelContent(scheduleRestore);
    const originalContentListener = originalEditor?.onDidChangeModelContent(scheduleRestore);
    scheduleRestore();
    return () => {
      modifiedContentListener.dispose();
      originalContentListener?.dispose();
      if (restoreFrameRef.current !== null) cancelAnimationFrame(restoreFrameRef.current);
      restoreFrameRef.current = null;
    };
  }, [diffEditorRef, modelKey, modifiedContent, originalContent, ready]);

  return useCallback(() => {
    if (restoreFrameRef.current !== null) cancelAnimationFrame(restoreFrameRef.current);
    restoreFrameRef.current = null;
    pendingRestoreRef.current = null;
  }, []);
}

interface MonacoDiffViewerProps {
  data: FileDiffData;
  sessionId?: string;
  enableComments?: boolean;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  onCommentUpdate?: (commentId: string, updates: DiffCommentUpdate) => void;
  onCommentRun?: (comment: DiffComment) => void;
  comments?: DiffComment[];
  className?: string;
  compact?: boolean;
  hideHeader?: boolean;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  wordWrap?: boolean;
  editable?: boolean;
  onModifiedContentChange?: (filePath: string, content: string) => void;
  repo?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  status?: string | null;
  previousPath?: string | null;
  publishedBranch?: string | null;
  externalBaseBranch?: string | null;
}

function useMonacoDiffViewerState(props: MonacoDiffViewerProps) {
  const {
    data,
    sessionId,
    compact = false,
    enableComments,
    onCommentAdd,
    onCommentDelete,
    onCommentUpdate,
    onCommentRun,
    comments: externalComments,
    onModifiedContentChange,
    wordWrap: wordWrapProp,
    editable,
    onRevert,
  } = props;
  const { resolvedTheme } = useTheme();
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const [foldUnchanged, setFoldUnchanged] = useGlobalFolding();
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const wrapperRef = useRef<HTMLDivElement>(null);
  const { setOpen: setCommandPanelOpen } = useCommandPanelOpen();

  const commentState = useDiffViewerComments({
    data,
    sessionId,
    repositoryName: props.repo,
    compact,
    enableComments,
    onCommentAdd,
    onCommentDelete,
    onCommentUpdate,
    onCommentRun,
    externalComments,
    onModifiedContentChange,
  });

  useCaptureKeydown(wrapperRef, { metaOrCtrl: true, key: "k" }, () => setCommandPanelOpen(true));

  useLayoutEffect(() => {
    const ref = commentState.diffEditorRef;
    return () => {
      try {
        ref.current?.setModel(null);
      } catch {
        /* already disposed */
      }
    };
  }, [commentState.diffEditorRef]);

  const { oldContent, newContent, diff, filePath } = data;
  const language = getMonacoLanguage(filePath);
  const { original, modified } = useMemo(
    () => resolveDiffContent({ oldContent, newContent, diff }),
    [oldContent, newContent, diff],
  );
  const lineHeight = compact ? 16 : 18;
  const editorHeight = useDiffEditorHeight({
    modifiedEditor: commentState.modifiedEditor,
    originalEditor: commentState.originalEditor,
    compact,
    lineHeight,
    originalContent: original,
    modifiedContent: modified,
  });
  const modelKey = `${props.repo ?? ""}\u0000${filePath}\u0000${djb2Hash(original)}\u0000${djb2Hash(modified)}`;
  const cancelPendingViewStateRestore = useMonacoDiffViewState(
    modelKey,
    commentState.diffEditorRef,
    !!commentState.modifiedEditor,
    original,
    modified,
  );

  return {
    resolvedTheme,
    globalViewMode,
    setGlobalViewMode,
    foldUnchanged,
    setFoldUnchanged,
    wordWrap,
    setWordWrap,
    wrapperRef,
    ...commentState,
    diff,
    filePath,
    language,
    original,
    modified,
    lineHeight,
    editorHeight,
    cancelPendingViewStateRestore,
    hasDiff: !!(oldContent || newContent || diff),
    monacoTheme: getMonacoTheme(resolvedTheme),
    options: buildDiffEditorOptions({
      compact,
      wordWrap,
      modifiedReadOnly: compact || (!editable && !onRevert),
      onRevert,
      globalViewMode,
      foldUnchanged,
      lineHeight,
    }),
  };
}

export function MonacoDiffViewer(props: MonacoDiffViewerProps) {
  const { t } = useTranslation();
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
        {t("editors:noDiffAvailable")}
      </div>
    );
  }

  return (
    <div
      ref={wrapperRef}
      className={cn("monaco-diff-viewer relative", className)}
      onWheelCapture={state.cancelPendingViewStateRestore}
      onTouchMoveCapture={state.cancelPendingViewStateRestore}
      onKeyDownCapture={state.cancelPendingViewStateRestore}
    >
      {showHeader && (
        <DiffViewerToolbar
          data={props.data}
          foldUnchanged={state.foldUnchanged}
          setFoldUnchanged={state.setFoldUnchanged}
          wordWrap={state.wordWrap}
          setWordWrap={state.setWordWrap}
          globalViewMode={state.globalViewMode}
          setGlobalViewMode={state.setGlobalViewMode}
          onCopyDiff={() => void copyToClipboard(state.diff ?? "")}
          onOpenFile={onOpenFile}
          onRevert={onRevert}
          sessionId={props.sessionId}
          taskId={props.taskId}
          repositoryId={props.repositoryId}
          repositoryName={props.repo}
          status={props.status}
          previousPath={props.previousPath}
          publishedBranch={props.publishedBranch}
          baseBranch={props.externalBaseBranch}
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
              {t("editors:loadingDiff")}
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
