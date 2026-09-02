"use client";

import type { ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconDeviceFloppy,
  IconLoader2,
  IconTrash,
  IconTextWrap,
  IconTextWrapDisabled,
  IconMessagePlus,
  IconArrowsDiff,
  IconRefresh,
  IconEye,
} from "@tabler/icons-react";
import { formatDiffStats } from "@/lib/utils/file-diff";
import { toRelativePath } from "@/lib/utils";
import { FileActionsDropdown } from "@/components/editors/file-actions-dropdown";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { LspStatusButton } from "@/components/editors/lsp-status-button";
import type { LspStatus } from "@/lib/lsp/lsp-client-manager";
import type { LspProgressSnapshot } from "@/lib/lsp/lsp-progress";
import { useTranslation } from "react-i18next";

const SAVE_SHORTCUT =
  typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "\u2318" : "Ctrl";

function SaveButton({
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
      className="cursor-pointer gap-2"
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

function ToolbarLeft({
  path,
  worktreePath,
  isDirty,
  diffStats,
}: {
  path: string;
  worktreePath?: string;
  isDirty: boolean;
  diffStats: { additions: number; deletions: number } | null;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
      <ScrollOnOverflow className="min-w-0 font-mono">
        {toRelativePath(path, worktreePath)}
      </ScrollOnOverflow>
      {isDirty && diffStats && (
        <span className="shrink-0 text-xs text-yellow-500">
          {formatDiffStats(diffStats.additions, diffStats.deletions)}
        </span>
      )}
    </div>
  );
}

function CommentCountBadge({
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

function DiffIndicatorsButton({
  isVisible,
  onToggle,
}: {
  isVisible: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const buttonClass = `h-8 w-8 p-0 cursor-pointer ${isVisible ? "text-foreground" : "text-muted-foreground"}`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button size="sm" variant="ghost" onClick={onToggle} className={buttonClass}>
          <IconArrowsDiff className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {isVisible ? t("editors:hideDiffIndicators") : t("editors:showDiffIndicators")}
      </TooltipContent>
    </Tooltip>
  );
}

function WrapButton({
  wrapEnabled,
  onToggleWrap,
}: {
  wrapEnabled: boolean;
  onToggleWrap: () => void;
}) {
  const { t } = useTranslation();
  const wrapClass = `h-8 w-8 p-0 cursor-pointer ${wrapEnabled ? "text-foreground" : "text-muted-foreground"}`;
  const wrapIcon = wrapEnabled ? (
    <IconTextWrap className="h-4 w-4" />
  ) : (
    <IconTextWrapDisabled className="h-4 w-4" />
  );
  const wrapLabel = wrapEnabled ? t("editors:disableWordWrap") : t("editors:enableWordWrap");

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button size="sm" variant="ghost" onClick={onToggleWrap} className={wrapClass}>
          {wrapIcon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{wrapLabel}</TooltipContent>
    </Tooltip>
  );
}

function ReloadFromAgentButton({
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
          className="h-8 cursor-pointer gap-1 px-2 text-xs"
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

function DeleteButton({ onDelete }: { onDelete?: () => void }) {
  const { t } = useTranslation();
  if (!onDelete) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onDelete}
          className="h-8 w-8 p-0 cursor-pointer hover:text-destructive"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:deleteFile")}</TooltipContent>
    </Tooltip>
  );
}

function MarkdownPreviewButton({ onTogglePreview }: { onTogglePreview: () => void }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onTogglePreview}
          className="h-8 w-8 p-0 cursor-pointer"
          data-testid="markdown-preview-toggle"
        >
          <IconEye className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{t("editors:previewMarkdown")}</TooltipContent>
    </Tooltip>
  );
}

interface MonacoEditorToolbarProps {
  path: string;
  repositoryName?: string;
  worktreePath?: string;
  isDirty: boolean;
  isSaving: boolean;
  diffStats: { additions: number; deletions: number } | null;
  wrapEnabled: boolean;
  showDiffIndicators: boolean;
  enableComments: boolean;
  sessionId?: string;
  commentCount: number;
  hasRemoteUpdate?: boolean;
  hasVcsDiff?: boolean;
  lspStatus: LspStatus;
  lspProgress: LspProgressSnapshot;
  lspLanguage: string | null;
  showLspStatus?: boolean;
  onToggleLsp: () => void;
  onToggleWrap: () => void;
  onToggleDiffIndicators: () => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
  onToggleMarkdownPreview?: () => void;
  toolbarModeControl?: ReactNode;
}

export function MonacoEditorToolbar({
  path,
  repositoryName,
  worktreePath,
  isDirty,
  isSaving,
  diffStats,
  wrapEnabled,
  showDiffIndicators,
  enableComments,
  sessionId,
  commentCount,
  hasRemoteUpdate = false,
  hasVcsDiff = false,
  lspStatus,
  lspProgress,
  lspLanguage,
  showLspStatus = true,
  onToggleLsp,
  onToggleWrap,
  onToggleDiffIndicators,
  onSave,
  onReloadFromAgent,
  onDelete,
  onToggleMarkdownPreview,
  toolbarModeControl,
}: MonacoEditorToolbarProps) {
  const fileStatus = useExternalVcsFileStatus(path, sessionId, repositoryName);
  return (
    <PanelHeaderBarSplit
      className={toolbarModeControl ? "markdown-file-toolbar" : undefined}
      left={
        <ToolbarLeft
          path={path}
          worktreePath={worktreePath}
          isDirty={isDirty}
          diffStats={diffStats}
        />
      }
      right={
        <div className="flex items-center gap-1">
          {toolbarModeControl}
          <CommentCountBadge
            enableComments={enableComments}
            sessionId={sessionId}
            commentCount={commentCount}
          />
          {showLspStatus ? (
            <LspStatusButton
              status={lspStatus}
              progress={lspProgress}
              lspLanguage={lspLanguage}
              onToggle={onToggleLsp}
            />
          ) : null}
          {(isDirty || hasVcsDiff) && (
            <DiffIndicatorsButton
              isVisible={showDiffIndicators}
              onToggle={onToggleDiffIndicators}
            />
          )}
          {onToggleMarkdownPreview && (
            <MarkdownPreviewButton onTogglePreview={onToggleMarkdownPreview} />
          )}
          <WrapButton wrapEnabled={wrapEnabled} onToggleWrap={onToggleWrap} />
          <ReloadFromAgentButton
            hasRemoteUpdate={hasRemoteUpdate}
            onReloadFromAgent={onReloadFromAgent}
          />
          <ExternalVcsFileLink
            filePath={path}
            previousPath={fileStatus?.old_path}
            status={fileStatus?.status}
            sessionId={sessionId}
            repositoryName={repositoryName}
            size="sm"
          />
          <FileActionsDropdown filePath={path} sessionId={sessionId} size="sm" />
          <DeleteButton onDelete={onDelete} />
          <SaveButton isDirty={isDirty} isSaving={isSaving} onSave={onSave} />
        </div>
      }
    />
  );
}
