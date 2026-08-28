"use client";

import { useCallback, useEffect, useState } from "react";
import { IconDeviceFloppy, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { FileViewerExternalLink } from "./file-viewer-header";
import { FileEditorContent } from "./file-editor-content";
import { MarkdownPreviewContent } from "./markdown-preview-content";
import {
  HybridMarkdownEditor,
  type MarkdownComment,
  type MarkdownGutterMarker,
} from "@/components/editors/markdown/hybrid-markdown-editor";
import {
  capitalize,
  isMarkdownFileModeSupported,
  type MarkdownFileMode,
} from "./markdown-file-mode";
import { useMarkdownEditorCommentState } from "./markdown-editor-comment-bridge";
import { useTranslation } from "react-i18next";

export {
  sourceLineEndOffset,
  sourceLinesAtOffsets,
  sourceOffsetAtLine,
} from "./markdown-editor-comment-bridge";

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
  onOpenFile?: (path: string) => void;
  onOpenLink?: (url: string) => void;
  onComment?: (comment: { text: string; start: number; endExclusive: number }) => void;
  onError?: (error: unknown) => void;
  onSourceFallback?: () => void;
};

const MODE_ORDER: readonly MarkdownFileMode[] = ["preview", "edit", "source"];

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
  onOpenFile,
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
  const [previewMounted, setPreviewMounted] = useState(mode === "preview");
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
    if (safeMode === "preview") setPreviewMounted(true);
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
      keepPreviewMounted={previewMounted}
      onModeChange={onModeChange}
      onChange={onChange}
      onSave={onSave}
      onReloadFromAgent={onReloadFromAgent}
      onDelete={onDelete}
      onOpenFile={onOpenFile}
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
  keepPreviewMounted: boolean;
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
  keepPreviewMounted,
  onModeChange,
  onChange,
  onSave,
  onReloadFromAgent,
  onDelete,
  onOpenFile,
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
        keepPreviewMounted={keepPreviewMounted}
        onChange={onChange}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
        onOpenFile={onOpenFile}
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
  | "onOpenFile"
  | "onOpenLink"
  | "onComment"
  | "onError"
  | "onSourceFallback"
> & { keepHybridMounted: boolean; keepPreviewMounted: boolean; mode: MarkdownFileMode };

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
  keepPreviewMounted,
  onChange,
  onSave,
  onReloadFromAgent,
  onDelete,
  onOpenFile,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
}: MarkdownFilePresentationProps) {
  return (
    <div className="min-h-0 flex-1">
      {keepHybridMounted && (
        <div
          className={mode === "edit" ? "h-full min-h-0 overflow-hidden" : "hidden"}
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
      {keepPreviewMounted && (
        <div
          className={mode === "preview" ? "h-full min-h-0" : "hidden"}
          aria-hidden={mode !== "preview"}
          data-testid="markdown-preview-host"
        >
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
            onOpenFile={onOpenFile}
            onOpenLink={onOpenLink}
          />
        </div>
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
