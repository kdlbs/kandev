'use client';

import { memo, useCallback } from 'react';
import {
  IconSettings,
  IconX,
  IconMessageForward,
  IconLayoutColumns,
  IconLayoutRows,
  IconTextWrap,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Checkbox } from '@kandev/ui/checkbox';
import type { DiffComment } from '@/lib/diff/types';
import { useAppStore } from '@/components/state-provider';
import { getWebSocketClient } from '@/lib/ws/connection';
import { updateUserSettings } from '@/lib/api';
import { VcsSplitButton } from '@/components/vcs-split-button';

type ReviewTopBarProps = {
  sessionId: string;
  reviewedCount: number;
  totalCount: number;
  commentCount: number;
  baseBranch?: string;
  splitView: boolean;
  onToggleSplitView: (split: boolean) => void;
  wordWrap: boolean;
  onToggleWordWrap: (wrap: boolean) => void;
  onSendComments: (comments: DiffComment[]) => void;
  onClose: () => void;
  getPendingComments: () => DiffComment[];
  markCommentsSent: (ids: string[]) => void;
};

function sendAutoMarkSetting(checked: boolean) {
  const payload = { review_auto_mark_on_scroll: checked };
  const client = getWebSocketClient();
  if (client) {
    client.request('user.settings.update', payload).catch(() => { updateUserSettings(payload, { cache: 'no-store' }).catch(() => {}); });
  } else {
    updateUserSettings(payload, { cache: 'no-store' }).catch(() => {});
  }
}

type ReviewSettingsMenuProps = { reviewAutoMarkOnScroll: boolean; onToggleAutoMark: (checked: boolean) => void };

function ReviewSettingsMenu({ reviewAutoMarkOnScroll, onToggleAutoMark }: ReviewSettingsMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild><Button size="sm" variant="ghost" className="px-2 cursor-pointer"><IconSettings className="h-4 w-4" /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64">
        <DropdownMenuItem className="cursor-pointer gap-2" onSelect={(e) => { e.preventDefault(); onToggleAutoMark(!reviewAutoMarkOnScroll); }}>
          <Checkbox checked={reviewAutoMarkOnScroll} className="pointer-events-none" />
          <span className="text-sm flex-1">Auto-mark reviewed on scroll</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type ReviewProgressProps = { reviewedCount: number; totalCount: number };

function ReviewProgress({ reviewedCount, totalCount }: ReviewProgressProps) {
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;
  return (
    <div className="flex items-center gap-2 flex-1 min-w-0">
      <div className="flex-1 h-2 rounded-full bg-muted overflow-hidden max-w-[200px]">
        <div className="h-full bg-emerald-500 rounded-full transition-all duration-300" style={{ width: `${progressPercent}%` }} />
      </div>
      <span className="text-xs text-muted-foreground whitespace-nowrap">{reviewedCount} of {totalCount} files reviewed</span>
    </div>
  );
}

export const ReviewTopBar = memo(function ReviewTopBar({
  sessionId, reviewedCount, totalCount, commentCount, baseBranch,
  splitView, onToggleSplitView, wordWrap, onToggleWordWrap,
  onSendComments, onClose, getPendingComments, markCommentsSent,
}: ReviewTopBarProps) {
  const reviewAutoMarkOnScroll = useAppStore((state) => state.userSettings.reviewAutoMarkOnScroll);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);

  const handleFixComments = useCallback(() => {
    const comments = getPendingComments();
    if (comments.length === 0) return;
    onSendComments(comments);
    markCommentsSent(comments.map((c) => c.id));
  }, [getPendingComments, onSendComments, markCommentsSent]);

  const handleToggleAutoMark = useCallback((checked: boolean) => {
    setUserSettings({ ...userSettings, reviewAutoMarkOnScroll: checked });
    sendAutoMarkSetting(checked);
  }, [userSettings, setUserSettings]);

  return (
    <div className="flex items-center gap-3 px-4 py-2 border-b border-border bg-card/50 min-h-[48px]">
      <ReviewSettingsMenu reviewAutoMarkOnScroll={reviewAutoMarkOnScroll} onToggleAutoMark={handleToggleAutoMark} />
      <ReviewProgress reviewedCount={reviewedCount} totalCount={totalCount} />
      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" className={`px-2 cursor-pointer ${wordWrap ? 'bg-muted' : ''}`} onClick={() => onToggleWordWrap(!wordWrap)}>
            <IconTextWrap className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Toggle word wrap</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button size="sm" variant="ghost" className="px-2 cursor-pointer" onClick={() => onToggleSplitView(!splitView)}>
            {splitView ? <IconLayoutRows className="h-4 w-4" /> : <IconLayoutColumns className="h-4 w-4" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{splitView ? 'Switch to unified view' : 'Switch to split view'}</TooltipContent>
      </Tooltip>
      {commentCount > 0 && (
        <Button size="sm" variant="outline" className="cursor-pointer" onClick={handleFixComments}>
          <IconMessageForward className="h-4 w-4" />Fix Comments
          <span className="ml-1 rounded-full bg-blue-500/10 px-1.5 py-0.5 text-xs font-medium text-blue-600 dark:text-blue-400">{commentCount}</span>
        </Button>
      )}
      <VcsSplitButton sessionId={sessionId} baseBranch={baseBranch} />
      <Button size="sm" variant="ghost" className="px-2 cursor-pointer" onClick={onClose}><IconX className="h-4 w-4" /></Button>
    </div>
  );
});
