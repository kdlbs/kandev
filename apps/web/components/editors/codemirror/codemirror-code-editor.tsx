"use client";

import { useRef } from "react";
import CodeMirror, { type ReactCodeMirrorRef } from "@uiw/react-codemirror";
import { Button } from "@kandev/ui/button";
import {
  IconDeviceFloppy,
  IconLoader2,
  IconTrash,
  IconTextWrap,
  IconTextWrapDisabled,
  IconMessagePlus,
  IconRefresh,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { formatDiffStats } from "@/lib/utils/file-diff";
import { toRelativePath } from "@/lib/utils";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import { EditorCommentPopover } from "@/components/task/editor-comment-popover";
import { CommentViewPopover } from "@/components/task/comment-view-popover";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { useCodeMirrorEditorState } from "./use-codemirror-editor-state";

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
  worktreePath?: string;
  enableComments?: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
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
  if (!enableComments || !sessionId || commentCount <= 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 text-xs text-primary">
      <IconMessagePlus className="h-3.5 w-3.5" />
      <span>
        {commentCount} comment{commentCount > 1 ? "s" : ""}
      </span>
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
      <TooltipContent>{wrapEnabled ? "Disable word wrap" : "Enable word wrap"}</TooltipContent>
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
          Reload
        </Button>
      </TooltipTrigger>
      <TooltipContent>Apply latest agent changes to this file</TooltipContent>
    </Tooltip>
  );
}

function CodeMirrorDeleteButton({ onDelete }: { onDelete?: () => void }) {
  if (!onDelete) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onDelete}
          className="h-8 w-8 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Delete file</TooltipContent>
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
          Saving...
        </>
      ) : (
        <>
          <IconDeviceFloppy className="h-4 w-4" />
          Save
          <span className="text-xs text-muted-foreground">({SAVE_SHORTCUT}+S)</span>
        </>
      )}
    </Button>
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
  commentCount,
  hasRemoteUpdate,
  onToggleWrap,
  onSave,
  onReloadFromAgent,
  onDelete,
}: {
  path: string;
  worktreePath?: string;
  isDirty: boolean;
  isSaving: boolean;
  diffStats: { additions: number; deletions: number } | null;
  wrapEnabled: boolean;
  enableComments: boolean;
  sessionId?: string;
  commentCount: number;
  hasRemoteUpdate?: boolean;
  onToggleWrap: () => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
}) {
  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
          {isDirty && diffStats && (
            <span className="text-xs text-yellow-500">
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
          <CodeMirrorWrapButton wrapEnabled={wrapEnabled} onToggleWrap={onToggleWrap} />
          <CodeMirrorReloadButton
            hasRemoteUpdate={hasRemoteUpdate}
            onReloadFromAgent={onReloadFromAgent}
          />
          <CodeMirrorDeleteButton onDelete={onDelete} />
          <CodeMirrorSaveButton isDirty={isDirty} isSaving={isSaving} onSave={onSave} />
        </div>
      }
    />
  );
}

export function CodeMirrorCodeEditor({
  path,
  content,
  originalContent,
  isDirty,
  hasRemoteUpdate = false,
  isSaving,
  sessionId,
  worktreePath,
  enableComments = false,
  onChange,
  onSave,
  onReloadFromAgent,
  onDelete,
}: FileEditorContentProps) {
  const wrapperRef = useRef<HTMLDivElement>(null);
  const editorRef = useRef<ReactCodeMirrorRef>(null);

  const state = useCodeMirrorEditorState({
    path,
    content,
    originalContent,
    isDirty,
    isSaving,
    sessionId,
    enableComments,
    onChange,
    onSave,
    wrapperRef,
    editorRef,
  });

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
        commentCount={state.comments.length}
        hasRemoteUpdate={hasRemoteUpdate}
        onToggleWrap={() => state.setWrapEnabled(!state.wrapEnabled)}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
      />

      <div className="flex-1 overflow-hidden relative">
        <CodeMirror
          ref={editorRef}
          value={content}
          height="100%"
          theme={vscodeDark}
          extensions={state.extensions}
          onChange={state.handleChange}
          basicSetup={{
            lineNumbers: true,
            foldGutter: true,
            highlightActiveLine: true,
            highlightSelectionMatches: true,
            searchKeymap: true,
          }}
          className="h-full overflow-auto text-xs"
        />

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
            Comment
          </Button>
        )}

        {state.textSelection && (
          <EditorCommentPopover
            selectedText={state.textSelection.text}
            lineRange={{ start: state.textSelection.startLine, end: state.textSelection.endLine }}
            position={state.textSelection.position}
            onSubmit={state.handleCommentSubmit}
            onClose={state.handlePopoverClose}
          />
        )}

        {state.commentView && (
          <CommentViewPopover
            comments={state.commentView.comments}
            position={state.commentView.position}
            onDelete={state.handleDeleteComment}
            onClose={state.handleCommentViewClose}
          />
        )}
      </div>
    </div>
  );
}
