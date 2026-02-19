"use client";

import { Button } from "@kandev/ui/button";
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
} from "@tabler/icons-react";
import { formatDiffStats } from "@/lib/utils/file-diff";
import { toRelativePath } from "@/lib/utils";
import { FileActionsDropdown } from "@/components/editors/file-actions-dropdown";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { LspStatusButton } from "@/components/editors/lsp-status-button";
import type { LspStatus } from "@/lib/lsp/lsp-client-manager";

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
          Saving...
        </>
      ) : (
        <>
          <IconDeviceFloppy className="h-4 w-4" />
          Save
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
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
      {isDirty && diffStats && (
        <span className="text-xs text-yellow-500">
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
  if (!enableComments || !sessionId || commentCount <= 0) return null;

  return (
    <div className="flex items-center gap-1 px-2 py-1 text-xs text-primary">
      <IconMessagePlus className="h-3.5 w-3.5" />
      <span>
        {commentCount} comment{commentCount > 1 ? "s" : ""}
      </span>
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
  const buttonClass = `h-8 w-8 p-0 cursor-pointer ${isVisible ? "text-foreground" : "text-muted-foreground"}`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button size="sm" variant="ghost" onClick={onToggle} className={buttonClass}>
          <IconArrowsDiff className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{isVisible ? "Hide diff indicators" : "Show diff indicators"}</TooltipContent>
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
  const wrapClass = `h-8 w-8 p-0 cursor-pointer ${wrapEnabled ? "text-foreground" : "text-muted-foreground"}`;
  const wrapIcon = wrapEnabled ? (
    <IconTextWrap className="h-4 w-4" />
  ) : (
    <IconTextWrapDisabled className="h-4 w-4" />
  );
  const wrapLabel = wrapEnabled ? "Disable word wrap" : "Enable word wrap";

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
          Reload
        </Button>
      </TooltipTrigger>
      <TooltipContent>Apply latest agent changes to this file</TooltipContent>
    </Tooltip>
  );
}

function DeleteButton({ onDelete }: { onDelete?: () => void }) {
  if (!onDelete) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          onClick={onDelete}
          className="h-8 w-8 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Delete file</TooltipContent>
    </Tooltip>
  );
}

interface MonacoEditorToolbarProps {
  path: string;
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
  lspLanguage: string | null;
  onToggleLsp: () => void;
  onToggleWrap: () => void;
  onToggleDiffIndicators: () => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
}

export function MonacoEditorToolbar({
  path,
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
  lspLanguage,
  onToggleLsp,
  onToggleWrap,
  onToggleDiffIndicators,
  onSave,
  onReloadFromAgent,
  onDelete,
}: MonacoEditorToolbarProps) {
  return (
    <PanelHeaderBarSplit
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
          <CommentCountBadge
            enableComments={enableComments}
            sessionId={sessionId}
            commentCount={commentCount}
          />
          <LspStatusButton status={lspStatus} lspLanguage={lspLanguage} onToggle={onToggleLsp} />
          {(isDirty || hasVcsDiff) && (
            <DiffIndicatorsButton
              isVisible={showDiffIndicators}
              onToggle={onToggleDiffIndicators}
            />
          )}
          <WrapButton wrapEnabled={wrapEnabled} onToggleWrap={onToggleWrap} />
          <ReloadFromAgentButton
            hasRemoteUpdate={hasRemoteUpdate}
            onReloadFromAgent={onReloadFromAgent}
          />
          <FileActionsDropdown filePath={path} sessionId={sessionId} size="sm" />
          <DeleteButton onDelete={onDelete} />
          <SaveButton isDirty={isDirty} isSaving={isSaving} onSave={onSave} />
        </div>
      }
    />
  );
}
