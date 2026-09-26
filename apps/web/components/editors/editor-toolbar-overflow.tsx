"use client";

import {
  IconArrowsDiff,
  IconDownload,
  IconEye,
  IconMessagePlus,
  IconRefresh,
  IconTextWrap,
  IconTrash,
} from "@tabler/icons-react";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";
import {
  ExternalVcsFileMenuItem,
  type ExternalVcsFileMenuItemProps,
} from "./external-vcs-file-link";
import { FileActionsMenuItems } from "./file-actions-dropdown";
import type { FilePreviewKind } from "@/lib/utils/file-types";
import { PanelHeaderOverflowMenu } from "@/components/task/panel-primitives";

export type EditorToolbarOverflowActionsProps = {
  filePath: string;
  previousPath?: string | null;
  status?: string | null;
  taskId?: string | null;
  sessionId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string | null;
  isDirty: boolean;
  hasVcsDiff?: boolean;
  showDiffIndicators?: boolean;
  onToggleDiffIndicators?: () => void;
  wrapEnabled: boolean;
  onToggleWrap: () => void;
  lspLanguage?: string | null;
  onToggleLsp?: () => void;
  hasRemoteUpdate?: boolean;
  onReloadFromAgent?: () => void;
  previewKind: FilePreviewKind;
  onTogglePreview?: () => void;
  onPreviewHtml?: () => void;
  isPublishingHtmlPreview?: boolean;
  commentCount?: number;
  enableComments?: boolean;
  onDownload?: () => void;
  onDelete?: () => void;
};

function EditorExternalVcsMenuItem({
  filePath,
  previousPath,
  status,
  taskId,
  sessionId,
  repositoryId,
  repositoryName,
}: Pick<
  EditorToolbarOverflowActionsProps,
  | "filePath"
  | "previousPath"
  | "status"
  | "taskId"
  | "sessionId"
  | "repositoryId"
  | "repositoryName"
>) {
  const input: ExternalVcsFileMenuItemProps = {
    filePath,
    previousPath,
    status,
    taskId,
    sessionId,
    repositoryId,
    repositoryName,
  };
  return <ExternalVcsFileMenuItem {...input} />;
}

function EditorLspOverflowItem({
  lspLanguage,
  onToggleLsp,
}: Pick<EditorToolbarOverflowActionsProps, "lspLanguage" | "onToggleLsp">) {
  const { t } = useTranslation();
  if (!lspLanguage || !onToggleLsp) return null;
  return (
    <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onToggleLsp}>
      {t("lsp:languageServerStatus")}
    </DropdownMenuItem>
  );
}

function EditorCommentOverflowItem({
  enableComments,
  commentCount,
}: Pick<EditorToolbarOverflowActionsProps, "enableComments" | "commentCount">) {
  const { t } = useTranslation();
  if (!enableComments || !commentCount) return null;
  return (
    <DropdownMenuItem disabled className="gap-2">
      <IconMessagePlus className="size-4" />
      {t("editors:commentCount", { count: commentCount })}
    </DropdownMenuItem>
  );
}

function EditorDiffOverflowItem({
  isDirty,
  hasVcsDiff,
  showDiffIndicators,
  onToggleDiffIndicators,
}: Pick<
  EditorToolbarOverflowActionsProps,
  "isDirty" | "hasVcsDiff" | "showDiffIndicators" | "onToggleDiffIndicators"
>) {
  const { t } = useTranslation();
  if (!(isDirty || hasVcsDiff) || !onToggleDiffIndicators) return null;
  return (
    <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onToggleDiffIndicators}>
      <IconArrowsDiff className="size-4" />
      {showDiffIndicators ? t("editors:hideDiffIndicators") : t("editors:showDiffIndicators")}
    </DropdownMenuItem>
  );
}

function EditorPreviewOverflowItem({
  previewKind,
  onTogglePreview,
  onPreviewHtml,
  isPublishingHtmlPreview,
}: Pick<
  EditorToolbarOverflowActionsProps,
  "previewKind" | "onTogglePreview" | "onPreviewHtml" | "isPublishingHtmlPreview"
>) {
  const { t } = useTranslation();
  const action = previewKind === "html" ? onPreviewHtml : onTogglePreview;
  if (previewKind === "none" || !action) return null;
  const label = previewKind === "html" ? t("editors:previewHtml") : t("editors:previewMarkdown");
  return (
    <DropdownMenuItem
      className="cursor-pointer gap-2"
      disabled={previewKind === "html" && isPublishingHtmlPreview}
      onSelect={action}
    >
      <IconEye className="size-4" />
      {label}
    </DropdownMenuItem>
  );
}

function EditorReloadOverflowItem({
  hasRemoteUpdate,
  onReloadFromAgent,
}: Pick<EditorToolbarOverflowActionsProps, "hasRemoteUpdate" | "onReloadFromAgent">) {
  const { t } = useTranslation();
  if (!hasRemoteUpdate || !onReloadFromAgent) return null;
  return (
    <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onReloadFromAgent}>
      <IconRefresh className="size-4" />
      {t("editors:reload")}
    </DropdownMenuItem>
  );
}

function EditorDownloadOverflowItem({
  onDownload,
}: Pick<EditorToolbarOverflowActionsProps, "onDownload">) {
  const { t } = useTranslation();
  if (!onDownload) return null;
  return (
    <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onDownload}>
      <IconDownload className="size-4" />
      {t("editors:downloadFile")}
    </DropdownMenuItem>
  );
}

function EditorDeleteOverflowItem({
  onDelete,
}: Pick<EditorToolbarOverflowActionsProps, "onDelete">) {
  const { t } = useTranslation();
  if (!onDelete) return null;
  return (
    <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onDelete}>
      <IconTrash className="size-4" />
      {t("editors:deleteFile")}
    </DropdownMenuItem>
  );
}

export function EditorToolbarOverflowActions({
  filePath,
  previousPath,
  status,
  taskId,
  sessionId,
  repositoryId,
  repositoryName,
  isDirty,
  hasVcsDiff = false,
  showDiffIndicators = false,
  onToggleDiffIndicators,
  wrapEnabled,
  onToggleWrap,
  lspLanguage,
  onToggleLsp,
  hasRemoteUpdate = false,
  onReloadFromAgent,
  previewKind,
  onTogglePreview,
  onPreviewHtml,
  isPublishingHtmlPreview = false,
  commentCount = 0,
  enableComments = false,
  onDownload,
  onDelete,
}: EditorToolbarOverflowActionsProps) {
  const { t } = useTranslation();
  return (
    <PanelHeaderOverflowMenu label={t("common:showMoreActions")}>
      <EditorLspOverflowItem lspLanguage={lspLanguage} onToggleLsp={onToggleLsp} />
      <EditorCommentOverflowItem enableComments={enableComments} commentCount={commentCount} />
      <EditorDiffOverflowItem
        isDirty={isDirty}
        hasVcsDiff={hasVcsDiff}
        showDiffIndicators={showDiffIndicators}
        onToggleDiffIndicators={onToggleDiffIndicators}
      />
      <EditorPreviewOverflowItem
        previewKind={previewKind}
        onTogglePreview={onTogglePreview}
        onPreviewHtml={onPreviewHtml}
        isPublishingHtmlPreview={isPublishingHtmlPreview}
      />
      <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onToggleWrap}>
        <IconTextWrap className="size-4" />
        {wrapEnabled ? t("editors:disableWordWrap") : t("editors:enableWordWrap")}
      </DropdownMenuItem>
      <EditorReloadOverflowItem
        hasRemoteUpdate={hasRemoteUpdate}
        onReloadFromAgent={onReloadFromAgent}
      />
      <EditorExternalVcsMenuItem
        filePath={filePath}
        previousPath={previousPath}
        status={status}
        taskId={taskId}
        sessionId={sessionId}
        repositoryId={repositoryId}
        repositoryName={repositoryName}
      />
      <FileActionsMenuItems filePath={filePath} sessionId={sessionId ?? undefined} />
      <EditorDownloadOverflowItem onDownload={onDownload} />
      <EditorDeleteOverflowItem onDelete={onDelete} />
    </PanelHeaderOverflowMenu>
  );
}
