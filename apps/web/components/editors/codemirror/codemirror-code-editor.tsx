"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import CodeMirror, { type ReactCodeMirrorRef } from "@uiw/react-codemirror";
import type { EditorView } from "@codemirror/view";
import { vscodeDark } from "@uiw/codemirror-theme-vscode";
import { EditorCommentPopover } from "@/components/task/editor-comment-popover";
import { CommentViewPopover } from "@/components/task/comment-view-popover";
import { registerCodeMirrorCursorRevealer } from "@/hooks/file-editor-cursor";
import { useCodeMirrorEditorState } from "./use-codemirror-editor-state";
import { useCodeMirrorWalkthroughRange } from "./use-codemirror-walkthrough-range";
import type { FilePreviewKind } from "@/lib/utils/file-types";
import { CodeMirrorToolbar } from "./codemirror-editor-toolbar";
import {
  clearCodeMirrorCursorFlash,
  codeMirrorCursorFlashExtension,
  revealCodeMirrorCursor,
  revealPendingCodeMirrorCursor,
} from "./codemirror-cursor-navigation";
import { EditorSelectionCommentTrigger } from "@/components/editors/editor-selection-comment-trigger";

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
  isSymlink?: boolean;
  repo?: string;
  enableComments?: boolean;
  previewKind?: FilePreviewKind;
  onTogglePreview?: () => void;
  onPreviewHtml?: () => void;
  isPublishingHtmlPreview?: boolean;
  onChange: (newContent: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  onDownload?: () => void;
};

type CodeMirrorEditorState = ReturnType<typeof useCodeMirrorEditorState>;

function CodeMirrorOverlays({
  state,
  hostRef,
}: {
  state: CodeMirrorEditorState;
  hostRef: RefObject<HTMLElement | null>;
}) {
  return (
    <>
      {state.floatingButtonPos && !state.textSelection && (
        <EditorSelectionCommentTrigger
          touch={state.touch}
          position={{ left: state.floatingButtonPos.x + 8, top: state.floatingButtonPos.y + 8 }}
          positioning="fixed"
          hostRef={hostRef}
          onOpen={() => state.handleFloatingButtonClick()}
        />
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
    repo,
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
    isSymlink,
    repo,
    enableComments = false,
    previewKind,
    onTogglePreview,
    onPreviewHtml,
    isPublishingHtmlPreview,
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
        isSymlink={isSymlink}
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
        previewKind={previewKind}
        onTogglePreview={onTogglePreview}
        onPreviewHtml={onPreviewHtml}
        isPublishingHtmlPreview={isPublishingHtmlPreview}
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
        <CodeMirrorOverlays state={state} hostRef={editorAreaRef} />
      </div>
    </div>
  );
}
