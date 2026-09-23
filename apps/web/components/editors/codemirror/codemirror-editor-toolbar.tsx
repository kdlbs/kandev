"use client";

import type { ReactNode } from "react";
import { SymlinkIndicator } from "@/components/shared/symlink-indicator";
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
import { EditorToolbarOverflowActions } from "@/components/editors/editor-toolbar-overflow";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import type { FilePreviewKind } from "@/lib/utils/file-types";
import { useTranslation } from "react-i18next";

const SAVE_SHORTCUT =
  typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "\u2318" : "Ctrl";

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
          className={`h-6 w-6 p-0 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11 ${wrapEnabled ? "text-foreground" : "text-muted-foreground"}`}
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
          className="h-6 min-h-6 cursor-pointer gap-1 px-2 text-xs max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
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
          className="h-6 w-6 p-0 cursor-pointer hover:text-destructive max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11"
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
          className="h-6 w-6 p-0 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11"
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
      className="min-h-6 cursor-pointer gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
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

function CodeMirrorPreviewButton({
  previewKind,
  onToggle,
  onPreviewHtml,
  isPublishingHtmlPreview,
}: {
  previewKind: FilePreviewKind;
  onToggle?: () => void;
  onPreviewHtml?: () => void;
  isPublishingHtmlPreview?: boolean;
}) {
  const { t } = useTranslation();
  if (previewKind === "none") return null;
  const isHtml = previewKind === "html";
  const action = isHtml ? onPreviewHtml : onToggle;
  if (!action) return null;
  const label = isHtml ? t("editors:previewHtml") : t("editors:previewMarkdown");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={action}
          disabled={isHtml && isPublishingHtmlPreview}
          aria-label={label}
          title={isHtml ? t("task:htmlPreviewTrustedCode") : undefined}
          className="h-6 w-6 p-0 cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11"
          data-testid={isHtml ? "html-preview-toggle" : "markdown-preview-toggle"}
        >
          {isHtml && isPublishingHtmlPreview ? (
            <IconLoader2 className="h-4 w-4 animate-spin" />
          ) : (
            <IconEye className="h-4 w-4" />
          )}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        <p>{label}</p>
        {isHtml && (
          <p className="mt-1 max-w-xs text-muted-foreground">{t("task:htmlPreviewTrustedCode")}</p>
        )}
      </TooltipContent>
    </Tooltip>
  );
}

function CodeMirrorFileLabel({
  path,
  worktreePath,
  isSymlink,
  isDirty,
  diffStats,
}: {
  path: string;
  worktreePath?: string;
  isSymlink?: boolean;
  isDirty: boolean;
  diffStats: { additions: number; deletions: number } | null;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
      <ScrollOnOverflow className="min-w-0 font-mono">
        {toRelativePath(path, worktreePath)}
      </ScrollOnOverflow>
      <SymlinkIndicator isSymlink={isSymlink} showLabel />
      {isDirty && diffStats && (
        <span className="shrink-0 text-xs text-yellow-500">
          {formatDiffStats(diffStats.additions, diffStats.deletions)}
        </span>
      )}
    </div>
  );
}

type CodeMirrorToolbarActionsProps = {
  path: string;
  taskId?: string | null;
  sessionId?: string;
  repositoryId?: string | null;
  repositoryName?: string;
  enableComments: boolean;
  commentCount: number;
  onTogglePreview?: () => void;
  onPreviewHtml?: () => void;
  isPublishingHtmlPreview?: boolean;
  previewKind?: FilePreviewKind;
  wrapEnabled: boolean;
  onToggleWrap: () => void;
  hasRemoteUpdate?: boolean;
  onReloadFromAgent?: () => void;
  onDownload?: () => void;
  onDelete?: () => void;
  isDirty: boolean;
  isSaving: boolean;
  onSave: () => void;
  toolbarModeControl?: ReactNode;
  fileStatus?: { old_path?: string | null; status?: string | null };
};

function CodeMirrorToolbarActions({
  path,
  taskId,
  sessionId,
  repositoryId,
  repositoryName,
  enableComments,
  commentCount,
  onTogglePreview,
  onPreviewHtml,
  isPublishingHtmlPreview,
  previewKind = "none",
  wrapEnabled,
  onToggleWrap,
  hasRemoteUpdate,
  onReloadFromAgent,
  onDownload,
  onDelete,
  isDirty,
  isSaving,
  onSave,
  toolbarModeControl,
  fileStatus,
}: CodeMirrorToolbarActionsProps) {
  return (
    <div className="flex items-center gap-1">
      {toolbarModeControl}
      <CodeMirrorCommentBadge
        enableComments={enableComments}
        sessionId={sessionId}
        commentCount={commentCount}
      />
      {(onTogglePreview || onPreviewHtml) && (
        <CodeMirrorPreviewButton
          previewKind={previewKind}
          onToggle={onTogglePreview}
          onPreviewHtml={onPreviewHtml}
          isPublishingHtmlPreview={isPublishingHtmlPreview}
        />
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
  );
}

/** Toolbar for the CodeMirror code editor. */
export type CodeMirrorToolbarProps = {
  path: string;
  worktreePath?: string;
  isSymlink?: boolean;
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
  previewKind?: FilePreviewKind;
  onTogglePreview?: () => void;
  onPreviewHtml?: () => void;
  isPublishingHtmlPreview?: boolean;
  toolbarModeControl?: ReactNode;
};

export function CodeMirrorToolbar(props: CodeMirrorToolbarProps) {
  const {
    path,
    worktreePath,
    isSymlink,
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
    previewKind = "none",
    onTogglePreview,
    onPreviewHtml,
    isPublishingHtmlPreview,
    toolbarModeControl,
  } = props;
  const fileStatus = useExternalVcsFileStatus(path, sessionId, repositoryName);
  const overflowActions = (
    <EditorToolbarOverflowActions
      filePath={path}
      previousPath={fileStatus?.old_path}
      status={fileStatus?.status}
      taskId={taskId}
      sessionId={sessionId}
      repositoryId={repositoryName ? undefined : repositoryId}
      repositoryName={repositoryName}
      isDirty={isDirty}
      wrapEnabled={wrapEnabled}
      onToggleWrap={onToggleWrap}
      hasRemoteUpdate={hasRemoteUpdate}
      onReloadFromAgent={onReloadFromAgent}
      previewKind={previewKind}
      onTogglePreview={onTogglePreview}
      onPreviewHtml={onPreviewHtml}
      isPublishingHtmlPreview={isPublishingHtmlPreview}
      commentCount={commentCount}
      enableComments={enableComments}
      onDownload={onDownload}
      onDelete={onDelete}
    />
  );
  return (
    <PanelHeaderBarSplit
      className={toolbarModeControl ? "markdown-file-toolbar" : undefined}
      left={
        <CodeMirrorFileLabel
          path={path}
          worktreePath={worktreePath}
          isSymlink={isSymlink}
          isDirty={isDirty}
          diffStats={diffStats}
        />
      }
      right={
        <CodeMirrorToolbarActions
          path={path}
          taskId={taskId}
          sessionId={sessionId}
          repositoryId={repositoryId}
          repositoryName={repositoryName}
          enableComments={enableComments}
          commentCount={commentCount}
          onTogglePreview={onTogglePreview}
          onPreviewHtml={onPreviewHtml}
          isPublishingHtmlPreview={isPublishingHtmlPreview}
          previewKind={previewKind}
          wrapEnabled={wrapEnabled}
          onToggleWrap={onToggleWrap}
          hasRemoteUpdate={hasRemoteUpdate}
          onReloadFromAgent={onReloadFromAgent}
          onDownload={onDownload}
          onDelete={onDelete}
          isDirty={isDirty}
          isSaving={isSaving}
          onSave={onSave}
          toolbarModeControl={toolbarModeControl}
          fileStatus={fileStatus}
        />
      }
      rightWhenOverflow={
        <CodeMirrorSaveButton isDirty={isDirty} isSaving={isSaving} onSave={onSave} />
      }
      overflow={overflowActions}
      overflowAt={520}
    />
  );
}
