"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import CodeMirror, { type ReactCodeMirrorRef } from "@uiw/react-codemirror";
import type { EditorView } from "@codemirror/view";
import { Button } from "@kandev/ui/button";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import {
  IconDeviceFloppy,
  IconLoader2,
  IconDownload,
  IconTrash,
  IconTextWrap,
  IconTextWrapDisabled,
  IconMessagePlus,
  IconRefresh,
  IconEye,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { formatDiffStats } from "@/lib/utils/file-diff";
import { toRelativePath } from "@/lib/utils";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import { EditorCommentPopover } from "@/components/task/editor-comment-popover";
import { CommentViewPopover } from "@/components/task/comment-view-popover";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import { registerCodeMirrorCursorRevealer } from "@/hooks/file-editor-cursor";
import { useCodeMirrorEditorState } from "./use-codemirror-editor-state";
import { useCodeMirrorWalkthroughRange } from "./use-codemirror-walkthrough-range";
import {
  clearCodeMirrorCursorFlash,
  codeMirrorCursorFlashExtension,
  revealCodeMirrorCursor,
  revealPendingCodeMirrorCursor,
} from "./codemirror-cursor-navigation";
import { useTranslation } from "react-i18next";

const SAVE_SHORTCUT =
  typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "\u2318" : "Ctrl";

type FileEditorContentProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  hasRemoteUpdate?: boolean;
  vcsDiff?: string;
  isSaving: boolean;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  worktreePath?: string;
  repo?: string;
  enableComments?: boolean;
  onToggleMarkdownPreview?: () => void;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  onDownload?: () => void;
};

function CodeMirrorCommentBadge({
  enableComments,
  sessionId,
  commentCount,
}: {
  enableComments: boolean;
  sessionId?: string;
  commentCount: number;
}) {
  const { t } = useTranslation();
  if (!enableComments || !sessionId || commentCount <= 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 text-xs text-primary">
      <IconMessagePlus className="h-3.5 w-3.5" />
      <span>{t("editors:commentCount", { count: commentCount })}</span>
    </div>
  );
}

function CodeMirrorWrapButton({
  wrapEnabled,
  onToggleWrap,
}: {
  wrapEnabled: boolean;
  onToggleWrap: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onToggleWrap}
          className={`h-8 w-8 p-0 cursor-pointer ${wrapEnabled ? "text-foreground" : "text-muted-foreground"}`}
        >
          {wrapEnabled ? (
            <IconTextWrap className="h-4 w-4" />
          ) : (
            <IconTextWrapDisabled className="h-4 w-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {wrapEnabled ? t("editors:disableWordWrap") : t("editors:enableWordWrap")}
      </TooltipContent>
    </Tooltip>
  );
}

function CodeMirrorReloadButton({
  hasRemoteUpdate,
  onReloadFromAgent,
}: {
  hasRemoteUpdate?: boolean;
  onReloadFromAgent?: () => void;
}) {
  const { t } = useTranslation();
  if (!hasRemoteUpdate || !onReloadFromAgent) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="outline"
          className="h-8 cursor-pointer gap-1 px-2 text-xs"
          onClick={onReloadFromAgent}
        >
          <IconRefresh className="h-3.5 w-3.5" />
          {t("editors:reload")}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:applyLatestAgentChangesToFile")}</TooltipContent>
    </Tooltip>
  );
}

function CodeMirrorDeleteButton({ onDelete }: { onDelete?: () => void }) {
  const { t } = useTranslation();
  if (!onDelete) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onDelete}
          className="h-8 w-8 p-0 cursor-pointer hover:text-destructive"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:deleteFile")}</TooltipContent>
    </Tooltip>
  );
}

function CodeMirrorDownloadButton({ onDownload }: { onDownload?: () => void }) {
  const { t } = useTranslation();
  if (!onDownload) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onDownload}
          aria-label={t("editors:downloadFile")}
          className="h-11 w-11 p-0 cursor-pointer sm:h-8 sm:w-8"
        >
          <IconDownload className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:downloadFile")}</TooltipContent>
    </Tooltip>
  );
}

function CodeMirrorSaveButton({
  isDirty,
  isSaving,
  onSave,
}: {
  isDirty: boolean;
  isSaving: boolean;
  onSave: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      size="sm"
      variant="default"
      onClick={onSave}
      disabled={!isDirty || isSaving}
      className="cursor-pointer gap-2"
    >
      {isSaving ? (
        <>
          <IconLoader2 className="h-4 w-4 animate-spin" />
          {t("editors:saving")}
        </>
      ) : (
        <>
          <IconDeviceFloppy className="h-4 w-4" />
          {t("common:save")}
          <span className="text-xs text-muted-foreground">({SAVE_SHORTCUT}+S)</span>
        </>
      )}
    </Button>
  );
}

function CodeMirrorMarkdownPreviewButton({ onToggle }: { onToggle: () => void }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onToggle}
          className="h-8 w-8 p-0 cursor-pointer"
          data-testid="markdown-preview-toggle"
        >
          <IconEye className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:previewMarkdown")}</TooltipContent>
    </Tooltip>
  );
}

/** Toolbar for the CodeMirror code editor. */
function CodeMirrorToolbar({
  path,
  worktreePath,
  isDirty,
  isSaving,
  diffStats,
  wrapEnabled,
  enableComments,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
  commentCount,
  hasRemoteUpdate,
  onToggleWrap,
  onSave,
  onReloadFromAgent,
  onDelete,
  onDownload,
  onToggleMarkdownPreview,
}: {
  path: string;
  worktreePath?: string;
  isDirty: boolean;
  isSaving: boolean;
  diffStats: { additions: number; deletions: number } | null;
  wrapEnabled: boolean;
  enableComments: boolean;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
  commentCount: number;
  hasRemoteUpdate?: boolean;
  onToggleWrap: () => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  onDownload?: () => void;
  onToggleMarkdownPreview?: () => void;
}) {
  const fileStatus = useExternalVcsFileStatus(path, sessionId, repositoryName);
  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <ScrollOnOverflow className="min-w-0 font-mono">
            {toRelativePath(path, worktreePath)}
          </ScrollOnOverflow>
          {isDirty && diffStats && (
            <span className="shrink-0 text-xs text-yellow-500">
              {formatDiffStats(diffStats.additions, diffStats.deletions)}
            </span>
          )}
        </div>
      }
      right={
        <div className="flex items-center gap-1">
          <CodeMirrorCommentBadge
            enableComments={enableComments}
            sessionId={sessionId}
            commentCount={commentCount}
          />
          {onToggleMarkdownPreview && (
            <CodeMirrorMarkdownPreviewButton onToggle={onToggleMarkdownPreview} />
          )}
          <CodeMirrorWrapButton wrapEnabled={wrapEnabled} onToggleWrap={onToggleWrap} />
          <CodeMirrorReloadButton
            hasRemoteUpdate={hasRemoteUpdate}
            onReloadFromAgent={onReloadFromAgent}
          />
          <ExternalVcsFileLink
            filePath={path}
            previousPath={fileStatus?.old_path}
            status={fileStatus?.status}
            taskId={taskId}
            sessionId={sessionId}
            repositoryId={repositoryName ? undefined : repositoryId}
            repositoryName={repositoryName}
            size="sm"
          />
          <CodeMirrorDownloadButton onDownload={onDownload} />
          <CodeMirrorDeleteButton onDelete={onDelete} />
          <CodeMirrorSaveButton isDirty={isDirty} isSaving={isSaving} onSave={onSave} />
        </div>
      }
    />
  );
}

type CodeMirrorEditorState = ReturnType<typeof useCodeMirrorEditorState>;

function CodeMirrorOverlays({ state }: { state: CodeMirrorEditorState }) {
  const { t } = useTranslation();
  return (
    <>
      {state.floatingButtonPos && !state.textSelection && (
        <Button
          size="sm"
          variant="secondary"
          className="floating-comment-btn fixed z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer"
          style={{ left: state.floatingButtonPos.x + 8, top: state.floatingButtonPos.y + 8 }}
          onMouseDown={(e) => e.stopPropagation()}
          onClick={state.handleFloatingButtonClick}
        >
          <IconMessagePlus className="h-3.5 w-3.5" />
          {t("editors:comment")}
        </Button>
      )}
      {state.textSelection && (
        <EditorCommentPopover
          selectedText={state.textSelection.text}
          lineRange={{ start: state.textSelection.startLine, end: state.textSelection.endLine }}
          position={state.textSelection.position}
          onSubmit={state.handleCommentSubmit}
          onSubmitAndRun={state.handleCommentSubmitAndRun}
          onClose={state.handlePopoverClose}
        />
      )}
      {state.commentView && (
        <CommentViewPopover
          comments={state.commentView.comments}
          position={state.commentView.position}
          onDelete={state.handleDeleteComment}
          onUpdate={state.handleUpdateComment}
          onClose={state.handleCommentViewClose}
        />
      )}
    </>
  );
}

function useCodeMirrorCodeEditorSetup(props: FileEditorContentProps) {
  const { path, content, originalContent, isDirty, isSaving, sessionId, repo, enableComments } =
    props;
  const wrapperRef = useRef<HTMLDivElement>(null);
  const editorAreaRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<ReactCodeMirrorRef>(null);
  const [editorView, setEditorView] = useState<EditorView | null>(null);
  const state = useCodeMirrorEditorState({
    path,
    content,
    originalContent,
    isDirty,
    isSaving,
    sessionId,
    enableComments: enableComments ?? false,
    onChange: props.onChange,
    onSave: props.onSave,
    wrapperRef,
    editorRef,
  });
  const walkthroughRange = useCodeMirrorWalkthroughRange({
    view: editorView,
    editorAreaRef,
    path,
    repo,
  });
  useEffect(() => {
    if (editorView) revealPendingCodeMirrorCursor(editorView, path, repo, sessionId);
  });
  useEffect(() => {
    if (!editorView) return;
    const unregister = registerCodeMirrorCursorRevealer(
      path,
      repo,
      sessionId,
      (line, column, options) => revealCodeMirrorCursor(editorView, line, column, options),
    );
    return () => {
      unregister();
      clearCodeMirrorCursorFlash(editorView);
    };
  }, [editorView, path, repo, sessionId]);
  const handleCreateEditor = useCallback((view: EditorView) => setEditorView(view), []);
  return { wrapperRef, editorAreaRef, editorRef, state, walkthroughRange, handleCreateEditor };
}

export function CodeMirrorCodeEditor(props: FileEditorContentProps) {
  const {
    path,
    content,
    isDirty,
    hasRemoteUpdate = false,
    isSaving,
    sessionId,
    taskId,
    repositoryId,
    worktreePath,
    repo,
    enableComments = false,
    onToggleMarkdownPreview,
    onSave,
    onReloadFromAgent,
    onDelete,
    onDownload,
  } = props;
  const { wrapperRef, editorAreaRef, editorRef, state, walkthroughRange, handleCreateEditor } =
    useCodeMirrorCodeEditorSetup(props);

  return (
    <div ref={wrapperRef} className="flex h-full flex-col rounded-lg">
      <CodeMirrorToolbar
        path={path}
        worktreePath={worktreePath}
        isDirty={isDirty}
        isSaving={isSaving}
        diffStats={state.diffStats}
        wrapEnabled={state.wrapEnabled}
        enableComments={enableComments}
        sessionId={sessionId}
        taskId={taskId}
        repositoryId={repositoryId}
        repositoryName={repo}
        commentCount={state.comments.length}
        hasRemoteUpdate={hasRemoteUpdate}
        onToggleWrap={() => state.setWrapEnabled(!state.wrapEnabled)}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
        onDownload={onDownload}
        onToggleMarkdownPreview={onToggleMarkdownPreview}
      />
      <div ref={editorAreaRef} className="flex-1 overflow-hidden relative">
        <CodeMirror
          ref={editorRef}
          value={content}
          height="100%"
          theme={vscodeDark}
          extensions={[...state.extensions, codeMirrorCursorFlashExtension]}
          onChange={state.handleChange}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            highlightActiveLine: true,
            highlightSelectionMatches: true,
            searchKeymap: true,
          }}
          onCreateEditor={handleCreateEditor}
          className="h-full overflow-auto text-xs"
        />
        {walkthroughRange ? (
          <div
            aria-hidden="true"
            data-testid="walkthrough-editor-range"
            data-line-range={`${walkthroughRange.startLine}-${walkthroughRange.endLine}`}
            className="pointer-events-none absolute z-20 rounded-sm border-l-2 border-primary/70 bg-primary/10 shadow-[inset_0_0_0_1px_hsl(var(--primary)/0.18)]"
            style={{
              top: walkthroughRange.top,
              left: walkthroughRange.left,
              width: walkthroughRange.width,
              height: walkthroughRange.height,
            }}
          />
        ) : null}
        <CodeMirrorOverlays state={state} />
      </div>
    </div>
  );
}
