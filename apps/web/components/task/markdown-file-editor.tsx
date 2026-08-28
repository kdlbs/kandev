"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import { IconDeviceFloppy, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
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
import { PanelHeaderBarSplit } from "./panel-primitives";
import { toRelativePath } from "@/lib/utils";
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
  onOpenLink?: (url: string) => boolean | void;
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
        supportedModes={supportedModes}
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
        onModeChange={onModeChange}
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
  | "onModeChange"
> & {
  keepHybridMounted: boolean;
  keepPreviewMounted: boolean;
  mode: MarkdownFileMode;
  supportedModes: readonly MarkdownFileMode[];
};

type MarkdownSurfaceProps = MarkdownFilePresentationProps & {
  modeControl: ReactNode;
  fileActions: ReactNode;
};

function MarkdownFilePresentation(props: MarkdownFilePresentationProps) {
  const modeControl = (
    <MarkdownModeControl
      mode={props.mode}
      supportedModes={props.supportedModes}
      onModeChange={props.onModeChange}
    />
  );
  const fileActions = (
    <MarkdownFileActions
      isDirty={props.isDirty}
      isSaving={props.isSaving}
      hasRemoteUpdate={props.hasRemoteUpdate ?? false}
      onSave={props.onSave}
      onReloadFromAgent={props.onReloadFromAgent}
      onDelete={props.onDelete}
    />
  );
  return (
    <div className="min-h-0 flex-1">
      <MarkdownEditSurface {...props} modeControl={modeControl} fileActions={fileActions} />
      <MarkdownPreviewSurface {...props} modeControl={modeControl} fileActions={fileActions} />
      <MarkdownSourceSurface {...props} modeControl={modeControl} fileActions={fileActions} />
    </div>
  );
}

function MarkdownEditSurface({
  mode,
  keepHybridMounted,
  path,
  worktreePath,
  modeControl,
  fileActions,
  sessionId,
  taskId,
  repositoryId,
  repo,
  content,
  originalContent,
  gutterMarkers,
  comments,
  onChange,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
}: MarkdownSurfaceProps) {
  if (!keepHybridMounted) return null;
  return (
    <div
      className={mode === "edit" ? "flex h-full min-h-0 flex-col overflow-hidden" : "hidden"}
      aria-hidden={mode !== "edit"}
      data-testid="markdown-hybrid-editor-host"
    >
      {mode === "edit" && (
        <MarkdownEditToolbar
          path={path}
          worktreePath={worktreePath}
          modeControl={modeControl}
          fileActions={fileActions}
          sessionId={sessionId}
          taskId={taskId}
          repositoryId={repositoryId}
          repositoryName={repo}
        />
      )}
      <div className="min-h-0 flex-1 overflow-hidden">
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
    </div>
  );
}

function MarkdownPreviewSurface({
  mode,
  keepPreviewMounted,
  path,
  content,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  repo,
  enableComments,
  onOpenFile,
  onOpenLink,
  modeControl,
  fileActions,
}: MarkdownSurfaceProps) {
  if (!keepPreviewMounted) return null;
  return (
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
        onOpenFile={onOpenFile}
        onOpenLink={onOpenLink}
        toolbarModeControl={modeControl}
        toolbarActions={fileActions}
        showToolbar={mode === "preview"}
      />
    </div>
  );
}

function MarkdownSourceSurface(props: MarkdownSurfaceProps) {
  if (props.mode !== "source") return null;
  return (
    <FileEditorContent
      path={props.path}
      content={props.content}
      originalContent={props.originalContent}
      isDirty={props.isDirty}
      hasRemoteUpdate={props.hasRemoteUpdate}
      vcsDiff={props.vcsDiff}
      isSaving={props.isSaving}
      sessionId={props.sessionId ?? undefined}
      taskId={props.taskId}
      repositoryId={props.repositoryId}
      worktreePath={props.worktreePath}
      repo={props.repo}
      enableComments={props.enableComments}
      markdownPreview={false}
      toolbarModeControl={props.modeControl}
      onChange={props.onChange}
      onSave={props.onSave}
      onReloadFromAgent={props.onReloadFromAgent}
      onDelete={props.onDelete}
    />
  );
}

type MarkdownModeControlProps = {
  mode: MarkdownFileMode;
  supportedModes: readonly MarkdownFileMode[];
  onModeChange: (mode: MarkdownFileMode) => void;
};

function MarkdownModeControl({ mode, supportedModes, onModeChange }: MarkdownModeControlProps) {
  const { t } = useTranslation();
  return (
    <div
      className="flex shrink-0 items-center gap-0.5"
      role="group"
      aria-label={t("task:markdownModes")}
    >
      {supportedModes.map((candidate) => (
        <Button
          key={candidate}
          type="button"
          size="sm"
          variant={candidate === mode ? "secondary" : "ghost"}
          className="h-8 cursor-pointer px-2 text-xs"
          data-testid={`markdown-mode-${candidate}`}
          aria-pressed={candidate === mode}
          onClick={() => onModeChange(candidate)}
        >
          {t(`task:markdownMode${capitalize(candidate)}`)}
        </Button>
      ))}
    </div>
  );
}

type MarkdownFileActionsProps = {
  isDirty: boolean;
  isSaving: boolean;
  hasRemoteUpdate: boolean;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
};

function MarkdownFileActions({
  isDirty,
  isSaving,
  hasRemoteUpdate,
  onSave,
  onReloadFromAgent,
  onDelete,
}: MarkdownFileActionsProps) {
  const { t } = useTranslation();
  return (
    <>
      {hasRemoteUpdate && onReloadFromAgent && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer gap-1 px-2 text-xs"
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
          className="h-8 w-8 cursor-pointer p-0"
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
        className="h-8 cursor-pointer gap-1 px-2 text-xs"
        disabled={!isDirty || isSaving}
        onClick={onSave}
        data-testid="markdown-file-save"
      >
        <IconDeviceFloppy className="h-3.5 w-3.5" />
        {isSaving ? t("task:saving") : t("common:save")}
      </Button>
    </>
  );
}

type MarkdownEditToolbarProps = {
  path: string;
  worktreePath?: string;
  modeControl: ReactNode;
  fileActions: ReactNode;
  sessionId?: string | null;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
};

function MarkdownEditToolbar({
  path,
  worktreePath,
  modeControl,
  fileActions,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
}: MarkdownEditToolbarProps) {
  return (
    <PanelHeaderBarSplit
      className="markdown-file-toolbar"
      left={
        <ScrollOnOverflow className="min-w-0 font-mono text-xs text-muted-foreground">
          {toRelativePath(path, worktreePath)}
        </ScrollOnOverflow>
      }
      right={
        <div className="flex items-center gap-1">
          {modeControl}
          <FileViewerExternalLink
            path={path}
            sessionId={sessionId}
            taskId={taskId}
            repositoryId={repositoryId}
            repositoryName={repositoryName}
          />
          {fileActions}
        </div>
      }
    />
  );
}
