"use client";

import { useEffect, useRef } from "react";
import Editor from "@monaco-editor/react";
import { useTheme } from "next-themes";
import { Button } from "@kandev/ui/button";
import { IconMessagePlus } from "@tabler/icons-react";
import { getMonacoLanguage } from "@/lib/editor/language-map";
import { EDITOR_FONT_FAMILY, EDITOR_FONT_SIZE } from "@/lib/theme/editor-theme";
import { CommentForm } from "@/components/diff/comment-form";
import { CommentDisplay } from "@/components/diff/comment-display";
import { useEditorViewZoneComments } from "@/hooks/use-editor-view-zone-comments";
import { MonacoEditorToolbar } from "./monaco-editor-toolbar";
import { useMonacoEditorComments } from "./use-monaco-editor-state";
import { useMonacoEditorLsp, useMonacoDiffDecorations } from "./use-monaco-editor-lsp";
import { initMonacoThemes } from "./monaco-init";

initMonacoThemes();

type MonacoCodeEditorProps = {
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

function getMonacoTheme(resolvedTheme: string | undefined): string {
  return resolvedTheme === "dark" ? "kandev-dark" : "kandev-light";
}

const EDITOR_OPTIONS = {
  fontSize: EDITOR_FONT_SIZE,
  fontFamily: EDITOR_FONT_FAMILY,
  lineHeight: 18,
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  smoothScrolling: true,
  cursorSmoothCaretAnimation: "on" as const,
  glyphMargin: false,
  lineDecorationsWidth: 10,
  folding: true,
  lineNumbers: "on" as const,
  renderLineHighlight: "line" as const,
  automaticLayout: true,
  scrollbar: { verticalScrollbarSize: 10, horizontalScrollbarSize: 10 },
  padding: { top: 4 },
  "semanticHighlighting.enabled": true,
};

type EditorState = ReturnType<typeof useMonacoEditorComments>;

function buildCommentZones(
  state: EditorState,
  addZone: (line: number, height: number, node: React.ReactNode) => void,
) {
  for (const comment of state.comments) {
    const isEditing = state.editingCommentId === comment.id;
    const node = isEditing ? (
      <div className="px-2 py-0.5" data-comment-zone>
        <CommentForm
          initialContent={comment.text}
          onSubmit={(c) => state.handleCommentUpdateRef.current(comment.id, c)}
          onCancel={() => state.setEditingComment(null)}
          isEditing
        />
      </div>
    ) : (
      <div className="px-2 py-0.5" data-comment-zone>
        <CommentDisplay
          comment={comment}
          onDelete={() => state.handleCommentDeleteRef.current(comment.id)}
          onEdit={() => state.setEditingComment(comment.id)}
          showCode={false}
          compact
        />
      </div>
    );
    addZone(comment.endLine, isEditing ? 120 : 32, node);
  }
  if (state.formZoneRange) {
    addZone(
      state.formZoneRange.endLine,
      120,
      <div className="px-2 py-1" data-comment-zone>
        <CommentForm
          onSubmit={(c) => state.handleCommentSubmitRef.current(c)}
          onCancel={() => {
            state.setFormZoneRange(null);
            state.clearGutterSelection();
          }}
        />
      </div>,
    );
  }
}

function useMonacoCodeEditorSetup(props: MonacoCodeEditorProps) {
  const {
    path,
    content,
    originalContent,
    isDirty,
    vcsDiff,
    sessionId,
    worktreePath,
    enableComments = false,
    onChange,
    onSave,
  } = props;
  const contentRef = useRef(content);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const language = getMonacoLanguage(path);
  const state = useMonacoEditorComments({
    path,
    enableComments,
    sessionId,
    wrapperRef,
    onChange,
    onSave,
    contentRef,
  });
  const lsp = useMonacoEditorLsp({
    sessionId,
    worktreePath,
    language,
    path,
    contentRef,
    editorRef: state.editorRef,
  });
  const { diffStats } = useMonacoDiffDecorations({
    originalContent,
    isDirty,
    showDiffIndicators: state.showDiffIndicators,
    vcsDiff,
    editorReady: state.editorInstance,
    contentRef,
    editorRef: state.editorRef,
    diffDecorationsRef: state.diffDecorationsRef,
  });
  useEffect(() => {
    contentRef.current = content;
  }, [content]);
  useEditorViewZoneComments(
    state.editorInstance,
    [state.comments, state.formZoneRange, state.editingCommentId],
    (addZone) => buildCommentZones(state, addZone),
  );
  const options = {
    ...EDITOR_OPTIONS,
    wordWrap: state.wrapEnabled ? ("on" as const) : ("off" as const),
  };
  return { contentRef, wrapperRef, language, state, lsp, diffStats, options };
}

export function MonacoCodeEditor(props: MonacoCodeEditorProps) {
  const {
    path,
    content,
    isDirty,
    hasRemoteUpdate = false,
    vcsDiff,
    isSaving,
    sessionId,
    worktreePath,
    enableComments = false,
    onSave,
    onReloadFromAgent,
    onDelete,
  } = props;
  const { resolvedTheme } = useTheme();
  const { wrapperRef, language, state, lsp, diffStats, options } = useMonacoCodeEditorSetup(props);

  return (
    <div ref={wrapperRef} className="flex h-full flex-col rounded-lg">
      <MonacoEditorToolbar
        path={path}
        worktreePath={worktreePath}
        isDirty={isDirty}
        isSaving={isSaving}
        diffStats={diffStats}
        wrapEnabled={state.wrapEnabled}
        showDiffIndicators={state.showDiffIndicators}
        enableComments={enableComments}
        sessionId={sessionId}
        commentCount={state.comments.length}
        hasRemoteUpdate={hasRemoteUpdate}
        hasVcsDiff={Boolean(vcsDiff)}
        lspStatus={lsp.lspStatus}
        lspLanguage={lsp.lspLanguage}
        onToggleLsp={lsp.toggleLsp}
        onToggleWrap={() => state.setWrapEnabled(!state.wrapEnabled)}
        onToggleDiffIndicators={() => state.setShowDiffIndicators(!state.showDiffIndicators)}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
      />
      <div className="flex-1 overflow-hidden relative">
        <Editor
          height="100%"
          language={language}
          path={lsp.monacoPath}
          value={content}
          theme={getMonacoTheme(resolvedTheme)}
          onChange={state.handleChange}
          onMount={state.handleEditorDidMount}
          keepCurrentModel
          options={options}
          loading={
            <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
              Loading editor...
            </div>
          }
        />
        {state.floatingButtonPos && !state.formZoneRange && (
          <Button
            size="sm"
            variant="secondary"
            className="floating-comment-btn absolute z-50 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer"
            style={{ left: state.floatingButtonPos.x + 4, top: state.floatingButtonPos.y + 2 }}
            onMouseDown={(e) => e.stopPropagation()}
            onClick={state.handleFloatingButtonClick}
          >
            <IconMessagePlus className="h-3.5 w-3.5" />
            Comment
          </Button>
        )}
      </div>
    </div>
  );
}
