"use client";

import { useCallback, useEffect } from "react";
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
import type { MarkdownFileMode } from "./markdown-file-mode";
import { isMarkdownFileModeSupported } from "./markdown-file-mode";
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

  useEffect(() => {
    if (safeMode !== mode) onModeChange(safeMode);
  }, [mode, onModeChange, safeMode]);

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        onSave();
      }
    },
    [onSave],
  );

  return (
    <div
      className="flex h-full min-h-0 flex-col"
      data-testid="markdown-file-editor"
      onKeyDown={handleKeyDown}
    >
      <MarkdownModeToolbar
        path={path}
        mode={safeMode}
        supportedModes={supportedModes}
        onModeChange={onModeChange}
        isDirty={isDirty}
        isSaving={isSaving}
        hasRemoteUpdate={hasRemoteUpdate}
        onSave={onSave}
        onReloadFromAgent={onReloadFromAgent}
        onDelete={onDelete}
        sessionId={sessionId}
        taskId={taskId}
        repositoryId={repositoryId}
        repositoryName={repo}
      />
      <MarkdownFilePresentation
        mode={safeMode}
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
        gutterMarkers={gutterMarkers}
        comments={comments}
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
> & { mode: MarkdownFileMode };

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
      {mode === "edit" && (
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

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}
