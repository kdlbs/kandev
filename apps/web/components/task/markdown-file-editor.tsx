"use client";

import { useCallback, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
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
import { useMarkdownFilePresentation } from "./use-markdown-file-presentation";
import {
  useMarkdownSourcePosition,
  type MarkdownSourcePositionPorts,
} from "./use-markdown-source-position";
import { MarkdownFileActions, MarkdownEditToolbar } from "./markdown-file-toolbar";
import { MarkdownHybridRecoveryStatus } from "./markdown-hybrid-recovery-status";
import { useMarkdownEditorCommentState } from "./markdown-editor-comment-bridge";
import { useMarkdownWalkthroughRange } from "./use-markdown-walkthrough-range";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";
import { useTranslation } from "react-i18next";

export {
  sourceLineEndOffset,
  sourceLinesAtOffsets,
  sourceOffsetAtLine,
} from "./markdown-editor-comment-bridge";

export type MarkdownFileEditorProps = {
  walkthroughTarget?: MarkdownWalkthroughTarget | null;
  draftIdentity?: symbol;
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

function useMarkdownEditorSaveShortcut(mode: MarkdownFileMode, onSave: () => void) {
  return useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (mode !== "edit") return;
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        onSave();
      }
    },
    [mode, onSave],
  );
}

export function MarkdownFileEditor({
  draftIdentity,
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
  const walkthroughTarget = useMarkdownWalkthroughRange({ path, repo, sessionId, taskId });
  const presentation = useMarkdownFilePresentation({ path, mode, onModeChange, onSourceFallback });
  const position = useMarkdownSourcePosition({
    identity: JSON.stringify([sessionId, repo, path]),
    draftIdentity,
    content,
    mode: presentation.safeMode,
    onModeChange: presentation.handleModeChange,
  });
  const handleSourceFallback = useCallback(() => {
    position.prepareModeChange("source");
    presentation.handleSourceFallback();
  }, [position.prepareModeChange, presentation.handleSourceFallback]);
  const { hybridComments, handleHybridComment } = useMarkdownEditorCommentState({
    path,
    content,
    sessionId,
    repositoryId: repo ? undefined : repositoryId,
    repositoryName: repo,
    enableComments,
    providedComments: comments,
    onComment,
  });

  const handleKeyDown = useMarkdownEditorSaveShortcut(presentation.safeMode, onSave);

  return (
    <MarkdownFileEditorLayout
      walkthroughTarget={walkthroughTarget}
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
      mode={presentation.safeMode}
      supportedModes={presentation.supportedModes}
      gutterMarkers={gutterMarkers}
      comments={hybridComments}
      keepHybridMounted={
        presentation.hybridMounted &&
        presentation.hybridAvailable &&
        !presentation.hybridFailed &&
        isMarkdownFileModeSupported(path, "edit")
      }
      hybridAvailable={presentation.hybridAvailable}
      hybridFailed={presentation.hybridFailed}
      keepPreviewMounted={presentation.previewMounted}
      sourcePositionHandles={position.sourcePositionHandles}
      onModeChange={position.handleModeChange}
      onChange={onChange}
      onSave={onSave}
      onReloadFromAgent={onReloadFromAgent}
      onDelete={onDelete}
      onOpenFile={onOpenFile}
      onOpenLink={onOpenLink}
      onComment={handleHybridComment}
      onError={onError}
      onSourceFallback={handleSourceFallback}
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
  hybridAvailable: boolean;
  hybridFailed: boolean;
  onKeyDown: (event: React.KeyboardEvent<HTMLDivElement>) => void;
  sourcePositionHandles: MarkdownSourcePositionPorts;
};

function MarkdownFileEditorLayout({
  walkthroughTarget,
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
  hybridAvailable,
  hybridFailed,
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
  sourcePositionHandles,
}: MarkdownFileEditorLayoutProps) {
  return (
    <div
      className="flex h-full min-h-0 flex-col"
      data-testid="markdown-file-editor"
      onKeyDown={onKeyDown}
    >
      <MarkdownFilePresentation
        walkthroughTarget={walkthroughTarget}
        sourcePositionHandles={sourcePositionHandles}
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
        hybridAvailable={hybridAvailable}
        hybridFailed={hybridFailed}
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

type MarkdownFilePresentationProps = { sourcePositionHandles: MarkdownSourcePositionPorts } & Pick<
  MarkdownFileEditorProps,
  | "walkthroughTarget"
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
    hybridAvailable: boolean;
    hybridFailed: boolean;
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
      hybridAvailable={props.hybridAvailable}
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
    <div className="flex h-full min-h-0 flex-1 flex-col">
      <MarkdownHybridRecoveryStatus
        unavailable={!props.hybridAvailable}
        failed={props.hybridFailed}
      />
      <div className="min-h-0 flex-1">
        <MarkdownEditSurface {...props} modeControl={modeControl} fileActions={fileActions} />
        <MarkdownPreviewSurface {...props} modeControl={modeControl} fileActions={fileActions} />
        <MarkdownSourceSurface {...props} modeControl={modeControl} fileActions={fileActions} />
      </div>
    </div>
  );
}

function MarkdownEditSurface({
  walkthroughTarget,
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
  gutterMarkers,
  comments,
  onChange,
  onOpenLink,
  onComment,
  onError,
  onSourceFallback,
  sourcePositionHandles,
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
          walkthroughTarget={walkthroughTarget}
          content={content}
          readOnly={false}
          gutterMarkers={gutterMarkers}
          comments={comments}
          onChange={onChange}
          onOpenLink={onOpenLink}
          onComment={onComment}
          onError={onError}
          onSourceFallback={onSourceFallback}
          isActive={mode === "edit"}
          onSourcePositionHandle={sourcePositionHandles.edit}
        />
      </div>
    </div>
  );
}

function MarkdownPreviewSurface({
  walkthroughTarget,
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
  sourcePositionHandles,
}: MarkdownSurfaceProps) {
  if (!keepPreviewMounted) return null;
  return (
    <div
      className={mode === "preview" ? "h-full min-h-0" : "hidden"}
      aria-hidden={mode !== "preview"}
      data-testid="markdown-preview-host"
    >
      <MarkdownPreviewContent
        walkthroughTarget={walkthroughTarget}
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
        isActive={mode === "preview"}
        onSourcePositionHandle={sourcePositionHandles.preview}
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
      onSourcePositionHandle={props.sourcePositionHandles.source}
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
  hybridAvailable: boolean;
  onModeChange: (mode: MarkdownFileMode) => void;
};

function MarkdownModeControl({
  mode,
  supportedModes,
  hybridAvailable,
  onModeChange,
}: MarkdownModeControlProps) {
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
          className="h-5 cursor-pointer rounded-sm px-1.5 text-xs"
          data-testid={`markdown-mode-${candidate}`}
          aria-pressed={candidate === mode}
          disabled={candidate === "edit" && !hybridAvailable}
          title={
            candidate === "edit" && !hybridAvailable ? t("task:markdownEditUnavailable") : undefined
          }
          onClick={() => onModeChange(candidate)}
        >
          {t(`task:markdownMode${capitalize(candidate)}`)}
        </Button>
      ))}
    </div>
  );
}
