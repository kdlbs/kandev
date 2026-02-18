'use client';

import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import {
  IconDeviceFloppy, IconLoader2, IconTrash,
  IconTextWrap, IconTextWrapDisabled,
  IconMessagePlus, IconArrowsDiff,
} from '@tabler/icons-react';
import { formatDiffStats } from '@/lib/utils/file-diff';
import { toRelativePath } from '@/lib/utils';
import { FileActionsDropdown } from '@/components/editors/file-actions-dropdown';
import { PanelHeaderBarSplit } from '@/components/task/panel-primitives';
import { LspStatusButton } from '@/components/editors/lsp-status-button';
import type { LspStatus } from '@/lib/lsp/lsp-client-manager';

const SAVE_SHORTCUT = typeof navigator !== 'undefined' && navigator.platform.includes('Mac') ? '\u2318' : 'Ctrl';

function SaveButton({ isDirty, isSaving, onSave }: { isDirty: boolean; isSaving: boolean; onSave: () => void }) {
  return (
    <Button size="sm" variant="default" onClick={onSave}
      disabled={!isDirty || isSaving} className="cursor-pointer gap-2">
      {isSaving ? (
        <><IconLoader2 className="h-4 w-4 animate-spin" />Saving...</>
      ) : (
        <>
          <IconDeviceFloppy className="h-4 w-4" />Save
          <span className="text-xs text-muted-foreground">({SAVE_SHORTCUT}+S)</span>
        </>
      )}
    </Button>
  );
}

function ToolbarLeft({ path, worktreePath, isDirty, diffStats }: {
  path: string; worktreePath?: string; isDirty: boolean;
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
  lspStatus: LspStatus;
  lspLanguage: string | null;
  onToggleLsp: () => void;
  onToggleWrap: () => void;
  onToggleDiffIndicators: () => void;
  onSave: () => void;
  onDelete?: () => void;
}

export function MonacoEditorToolbar({
  path, worktreePath, isDirty, isSaving, diffStats,
  wrapEnabled, showDiffIndicators, enableComments, sessionId,
  commentCount, lspStatus, lspLanguage,
  onToggleLsp, onToggleWrap, onToggleDiffIndicators, onSave, onDelete,
}: MonacoEditorToolbarProps) {
  const wrapIcon = wrapEnabled ? <IconTextWrap className="h-4 w-4" /> : <IconTextWrapDisabled className="h-4 w-4" />;
  const wrapLabel = wrapEnabled ? 'Disable word wrap' : 'Enable word wrap';
  const wrapClass = `h-8 w-8 p-0 cursor-pointer ${wrapEnabled ? 'text-foreground' : 'text-muted-foreground'}`;
  const diffClass = `h-8 w-8 p-0 cursor-pointer ${showDiffIndicators ? 'text-foreground' : 'text-muted-foreground'}`;

  return (
    <PanelHeaderBarSplit
      left={<ToolbarLeft path={path} worktreePath={worktreePath} isDirty={isDirty} diffStats={diffStats} />}
      right={
        <div className="flex items-center gap-1">
          {enableComments && sessionId && commentCount > 0 && (
            <div className="flex items-center gap-1 px-2 py-1 text-xs text-primary">
              <IconMessagePlus className="h-3.5 w-3.5" />
              <span>{commentCount} comment{commentCount > 1 ? 's' : ''}</span>
            </div>
          )}
          <LspStatusButton status={lspStatus} lspLanguage={lspLanguage} onToggle={onToggleLsp} />
          {isDirty && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" variant="ghost" onClick={onToggleDiffIndicators} className={diffClass}>
                  <IconArrowsDiff className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{showDiffIndicators ? 'Hide diff indicators' : 'Show diff indicators'}</TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="sm" variant="ghost" onClick={onToggleWrap} className={wrapClass}>
                {wrapIcon}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{wrapLabel}</TooltipContent>
          </Tooltip>
          <FileActionsDropdown filePath={path} sessionId={sessionId} size="sm" />
          {onDelete && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button size="sm" variant="ghost" onClick={onDelete}
                  className="h-8 w-8 p-0 cursor-pointer text-muted-foreground hover:text-destructive">
                  <IconTrash className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Delete file</TooltipContent>
            </Tooltip>
          )}
          <SaveButton isDirty={isDirty} isSaving={isSaving} onSave={onSave} />
        </div>
      }
    />
  );
}
