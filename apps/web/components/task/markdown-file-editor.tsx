"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconDeviceFloppy, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { FileViewerExternalLink } from "./file-viewer-header";
import { FileEditorContent } from "./file-editor-content";
import { MarkdownPreviewContent } from "./markdown-preview-content";
import {
  HybridMarkdownEditor,
  type MarkdownComment,
  type MarkdownCommentSubmission,
  type MarkdownGutterMarker,
} from "@/components/editors/markdown/hybrid-markdown-editor";
import { buildDiffComment } from "@/lib/diff/comment-utils";
import { useDiffFileComments } from "@/hooks/domains/comments/use-diff-comments";
import { useCommentsStore } from "@/lib/state/slices/comments";
import {
  capitalize,
  isMarkdownFileModeSupported,
  type MarkdownFileMode,
} from "./markdown-file-mode";
import { useTranslation } from "react-i18next";

export type MarkdownFileEditorProps = {
  path: string;
  content: string;
  originalContent: string;
  isDirty: boolean;
  hasRemoteUpdate?: boolean;
  vcsDiff?: string;
  isSaving: boolean;
  sessionId?: string | null;
  taskId?: string | null;
  repositoryId?: string | null;
  worktreePath?: string;
  repo?: string;
  enableComments?: boolean;
  mode: MarkdownFileMode;
  onModeChange: (mode: MarkdownFileMode) => void;
  onChange: (content: string) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  comments?: readonly MarkdownComment[];
  gutterMarkers?: readonly MarkdownGutterMarker[];
  onOpenLink?: (url: string) => void;
  onComment?: (comment: { text: string; start: number; endExclusive: number }) => void;
  onError?: (error: unknown) => void;
  onSourceFallback?: () => void;
};

const MODE_ORDER: readonly MarkdownFileMode[] = ["preview", "edit", "source"];

function clampSourceOffset(content: string, offset: number): number {
  return Math.max(0, Math.min(content.length, offset));
}

export function sourceOffsetAtLine(content: string, line: number): number {
  const targetLine = Math.max(1, line);
  if (targetLine === 1) return 0;
  let currentLine = 1;
  for (let offset = 0; offset < content.length; offset += 1) {
    if (content[offset] !== "\n") continue;
    currentLine += 1;
    if (currentLine === targetLine) return offset + 1;
  }
  return content.length;
}

export function sourceLineEndOffset(content: string, line: number): number {
  const start = sourceOffsetAtLine(content, line);
  const newline = content.indexOf("\n", start);
  if (newline === -1) return content.length;
  return newline > start && content[newline - 1] === "\r" ? newline - 1 : newline;
}

export function sourceLinesAtOffsets(
  content: string,
  start: number,
  endExclusive: number,
): { startLine: number; endLine: number; selectedText: string } {
  const safeStart = clampSourceOffset(content, start);
  const safeEnd = Math.max(safeStart, clampSourceOffset(content, endExclusive));
  const lineAt = (offset: number) => {
    let line = 1;
    for (let index = 0; index < offset; index += 1) {
      if (content[index] === "\n") line += 1;
    }
    return line;
  };
  return {
    startLine: lineAt(safeStart),
    endLine: lineAt(Math.max(safeStart, safeEnd - 1)),
    selectedText: content.slice(safeStart, safeEnd),
  };
}

function sourceCommentRange(content: string, startLine: number, endLine: number) {
  const start = sourceOffsetAtLine(content, startLine);
  const end = Math.max(start, sourceLineEndOffset(content, endLine));
  return { start, endExclusive: end };
}

function useMarkdownEditorCommentState({
  path,
  content,
  sessionId,
  repositoryId,
  enableComments,
  providedComments,
  onComment,
}: {
  path: string;
  content: string;
  sessionId?: string | null;
  repositoryId?: string | null;
  enableComments: boolean;
  providedComments?: readonly MarkdownComment[];
  onComment?: (comment: MarkdownCommentSubmission) => void;
}) {
  const fileComments = useDiffFileComments(sessionId ?? "", path, repositoryId ?? undefined);
  const addComment = useCommentsStore((state) => state.addComment);
  const hybridComments = useMemo<MarkdownComment[]>(
    () =>
      providedComments
        ? [...providedComments]
        : fileComments.map((comment) => {
            const range = sourceCommentRange(content, comment.startLine, comment.endLine);
            return {
              id: comment.id,
              start: range.start,
              endExclusive: range.endExclusive,
              body: comment.text,
            };
          }),
    [content, fileComments, providedComments],
  );

  const handleHybridComment = useCallback(
    (submission: MarkdownCommentSubmission) => {
      if (enableComments && sessionId) {
        const sourceLines = sourceLinesAtOffsets(
          content,
          submission.start,
          submission.endExclusive,
        );
        const comment = buildDiffComment({
          filePath: path,
          sessionId,
          startLine: sourceLines.startLine,
          endLine: sourceLines.endLine,
          side: "additions",
          text: submission.text,
          codeContent: sourceLines.selectedText,
        });
        if (repositoryId) comment.repositoryId = repositoryId;
        addComment(comment);
      }
      onComment?.(submission);
    },
    [addComment, content, enableComments, onComment, path, repositoryId, sessionId],
  );

  return { hybridComments, handleHybridComment };
}

export function MarkdownFileEditor({
  path,
  content,
  originalContent,
  isDirty,
  hasRemoteUpdate = false,
  vcsDiff,
  isSaving,
  sessionId,
  taskId,
  repositoryId,
  worktreePath,
  repo,
  enableComments = false,
  mode,
  onModeChange,
  onChange,
  onSave,
  onReloadFromAgent,
  onDelete,
  comments,
  gutterMarkers,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
}: MarkdownFileEditorProps) {
  const supportedModes = MODE_ORDER.filter((candidate) =>
    isMarkdownFileModeSupported(path, candidate),
  );
  const safeMode = supportedModes.includes(mode) ? mode : "source";
  const [hybridMounted, setHybridMounted] = useState(mode === "edit");
  const { hybridComments, handleHybridComment } = useMarkdownEditorCommentState({
    path,
    content,
    sessionId,
    repositoryId,
    enableComments,
    providedComments: comments,
    onComment,
  });

  useEffect(() => {
    if (safeMode !== mode) onModeChange(safeMode);
  }, [mode, onModeChange, safeMode]);

  useEffect(() => {
    if (safeMode === "edit") setHybridMounted(true);
  }, [safeMode]);

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (safeMode !== "edit") return;
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        onSave();
      }
    },
    [onSave, safeMode],
  );

  return (
    <MarkdownFileEditorLayout
      path={path}
      content={content}
      originalContent={originalContent}
      isDirty={isDirty}
      hasRemoteUpdate={hasRemoteUpdate}
      vcsDiff={vcsDiff}
      isSaving={isSaving}
      sessionId={sessionId}
      taskId={taskId}
      repositoryId={repositoryId}
      worktreePath={worktreePath}
      repo={repo}
      enableComments={enableComments}
      mode={safeMode}
      supportedModes={supportedModes}
      gutterMarkers={gutterMarkers}
      comments={hybridComments}
      keepHybridMounted={hybridMounted && isMarkdownFileModeSupported(path, "edit")}
      onModeChange={onModeChange}
      onChange={onChange}
      onSave={onSave}
      onReloadFromAgent={onReloadFromAgent}
      onDelete={onDelete}
      onOpenLink={onOpenLink}
      onComment={handleHybridComment}
      onError={onError}
      onSourceFallback={onSourceFallback}
      onKeyDown={handleKeyDown}
    />
  );
}

type MarkdownFileEditorLayoutProps = Omit<MarkdownFileEditorProps, "mode" | "comments"> & {
  mode: MarkdownFileMode;
  supportedModes: readonly MarkdownFileMode[];
  comments: readonly MarkdownComment[];
  keepHybridMounted: boolean;
  onKeyDown: (event: React.KeyboardEvent<HTMLDivElement>) => void;
};

function MarkdownFileEditorLayout({
  path,
  content,
  originalContent,
  isDirty,
  hasRemoteUpdate,
  vcsDiff,
  isSaving,
  sessionId,
  taskId,
  repositoryId,
  worktreePath,
  repo,
  enableComments,
  gutterMarkers,
  mode,
  supportedModes,
  comments,
  keepHybridMounted,
  onModeChange,
  onChange,
  onSave,
  onReloadFromAgent,
  onDelete,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
  onKeyDown,
}: MarkdownFileEditorLayoutProps) {
  return (
    <div
      className="flex h-full min-h-0 flex-col"
      data-testid="markdown-file-editor"
      onKeyDown={onKeyDown}
    >
      <MarkdownModeToolbar
        path={path}
        mode={mode}
        supportedModes={supportedModes}
        onModeChange={onModeChange}
        isDirty={isDirty}
        isSaving={isSaving}
        hasRemoteUpdate={hasRemoteUpdate ?? false}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
        sessionId={sessionId}
        taskId={taskId}
        repositoryId={repositoryId}
        repositoryName={repo}
      />
      <MarkdownFilePresentation
        mode={mode}
        path={path}
        content={content}
        originalContent={originalContent}
        isDirty={isDirty}
        hasRemoteUpdate={hasRemoteUpdate ?? false}
        vcsDiff={vcsDiff}
        isSaving={isSaving}
        sessionId={sessionId}
        taskId={taskId}
        repositoryId={repositoryId}
        worktreePath={worktreePath}
        repo={repo}
        enableComments={enableComments ?? false}
        gutterMarkers={gutterMarkers}
        comments={comments}
        keepHybridMounted={keepHybridMounted}
        onChange={onChange}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
        onOpenLink={onOpenLink}
        onComment={onComment}
        onError={onError}
        onSourceFallback={onSourceFallback}
      />
    </div>
  );
}

type MarkdownFilePresentationProps = Pick<
  MarkdownFileEditorProps,
  | "path"
  | "content"
  | "originalContent"
  | "isDirty"
  | "hasRemoteUpdate"
  | "vcsDiff"
  | "isSaving"
  | "sessionId"
  | "taskId"
  | "repositoryId"
  | "worktreePath"
  | "repo"
  | "enableComments"
  | "gutterMarkers"
  | "comments"
  | "onChange"
  | "onSave"
  | "onReloadFromAgent"
  | "onDelete"
  | "onOpenLink"
  | "onComment"
  | "onError"
  | "onSourceFallback"
> & { keepHybridMounted: boolean; mode: MarkdownFileMode };

function MarkdownFilePresentation({
  mode,
  path,
  content,
  originalContent,
  isDirty,
  hasRemoteUpdate,
  vcsDiff,
  isSaving,
  sessionId,
  taskId,
  repositoryId,
  worktreePath,
  repo,
  enableComments,
  gutterMarkers,
  comments,
  keepHybridMounted,
  onChange,
  onSave,
  onReloadFromAgent,
  onDelete,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
}: MarkdownFilePresentationProps) {
  return (
    <div className="min-h-0 flex-1">
      {keepHybridMounted && (
        <div
          className={mode === "edit" ? "h-full min-h-0" : "hidden"}
          aria-hidden={mode !== "edit"}
          data-testid="markdown-hybrid-editor-host"
        >
          <HybridMarkdownEditor
            content={content}
            baseline={originalContent}
            readOnly={false}
            gutterMarkers={gutterMarkers}
            comments={comments}
            onChange={onChange}
            onOpenLink={onOpenLink}
            onComment={onComment}
            onError={onError}
            onSourceFallback={onSourceFallback}
          />
        </div>
      )}
      {mode === "preview" && (
        <MarkdownPreviewContent
          path={path}
          content={content}
          worktreePath={worktreePath}
          sessionId={sessionId ?? undefined}
          taskId={taskId}
          repositoryId={repositoryId}
          repositoryName={repo}
          enableComments={enableComments}
          onTogglePreview={undefined}
        />
      )}
      {mode === "source" && (
        <FileEditorContent
          path={path}
          content={content}
          originalContent={originalContent}
          isDirty={isDirty}
          hasRemoteUpdate={hasRemoteUpdate}
          vcsDiff={vcsDiff}
          isSaving={isSaving}
          sessionId={sessionId ?? undefined}
          taskId={taskId}
          repositoryId={repositoryId}
          worktreePath={worktreePath}
          repo={repo}
          enableComments={enableComments}
          onChange={onChange}
          onSave={onSave}
          onReloadFromAgent={onReloadFromAgent}
          onDelete={onDelete}
        />
      )}
    </div>
  );
}

type MarkdownModeToolbarProps = {
  path: string;
  mode: MarkdownFileMode;
  supportedModes: readonly MarkdownFileMode[];
  onModeChange: (mode: MarkdownFileMode) => void;
  isDirty: boolean;
  isSaving: boolean;
  hasRemoteUpdate: boolean;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  sessionId?: string | null;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
};

function MarkdownModeToolbar({
  path,
  mode,
  supportedModes,
  onModeChange,
  isDirty,
  isSaving,
  hasRemoteUpdate,
  onSave,
  onReloadFromAgent,
  onDelete,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
}: MarkdownModeToolbarProps) {
  const { t } = useTranslation();
  return (
    <div
      className="flex min-h-11 shrink-0 items-center gap-2 border-b border-foreground/10 px-2"
      data-testid="markdown-mode-toolbar"
    >
      <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
        {path}
      </span>
      <div
        className="flex shrink-0 items-center gap-1"
        role="group"
        aria-label={t("task:markdownModes")}
      >
        {supportedModes.map((candidate) => (
          <Button
            key={candidate}
            type="button"
            size="sm"
            variant={candidate === mode ? "secondary" : "ghost"}
            className="h-9 cursor-pointer px-2 text-xs"
            data-testid={`markdown-mode-${candidate}`}
            aria-pressed={candidate === mode}
            onClick={() => onModeChange(candidate)}
          >
            {t(`task:markdownMode${capitalize(candidate)}`)}
          </Button>
        ))}
      </div>
      <FileViewerExternalLink
        path={path}
        sessionId={sessionId}
        taskId={taskId}
        repositoryId={repositoryId}
        repositoryName={repositoryName}
      />
      {hasRemoteUpdate && onReloadFromAgent && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-9 cursor-pointer gap-1 px-2 text-xs"
          onClick={onReloadFromAgent}
          data-testid="markdown-file-reload"
        >
          <IconRefresh className="h-3.5 w-3.5" />
          {t("common:reload")}
        </Button>
      )}
      {onDelete && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-9 w-9 cursor-pointer p-0"
          onClick={onDelete}
          aria-label={t("task:delete")}
          data-testid="markdown-file-delete"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      )}
      <Button
        type="button"
        variant="default"
        size="sm"
        className="h-9 cursor-pointer gap-1 px-2 text-xs"
        disabled={!isDirty || isSaving}
        onClick={onSave}
        data-testid="markdown-file-save"
      >
        <IconDeviceFloppy className="h-3.5 w-3.5" />
        {isSaving ? t("task:saving") : t("common:save")}
      </Button>
    </div>
  );
}
