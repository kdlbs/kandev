'use client';

import { memo } from 'react';
import {
  IconDeviceFloppy,
  IconX,
  IconLoader2,
  IconRobot,
  IconUser,
  IconArrowRight,
  IconCopy,
  IconRefresh,
  IconTrash,
  IconSend,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn, formatRelativeTime } from '@/lib/utils';
import type { TaskPlan } from '@/lib/types/http';

type TaskPlanToolbarProps = {
  plan: TaskPlan | null;
  hasUnsavedChanges: boolean;
  hasDraftContent: boolean;
  isSaving: boolean;
  isAgentBusy: boolean;
  isReanalyzing: boolean;
  hasActiveSession: boolean;
  showApproveButton?: boolean;
  // Comments props
  commentCount?: number;
  isSubmittingComments?: boolean;
  onSubmitComments?: () => void;
  onClearComments?: () => void;
  // Existing handlers
  onSave: () => void;
  onDiscard: () => void;
  onCopy: () => void;
  onReanalyze: () => void;
  onClear: () => void;
  onApprove?: () => void;
};

export const TaskPlanToolbar = memo(function TaskPlanToolbar({
  plan,
  hasUnsavedChanges,
  hasDraftContent,
  isSaving,
  isAgentBusy,
  isReanalyzing,
  hasActiveSession,
  showApproveButton,
  commentCount = 0,
  isSubmittingComments = false,
  onSubmitComments,
  onClearComments,
  onSave,
  onDiscard,
  onCopy,
  onReanalyze,
  onClear,
  onApprove,
}: TaskPlanToolbarProps) {
  const hasComments = commentCount > 0;
  return (
    <div className="mt-2">
      <div
        className={cn(
          'rounded-2xl border border-border bg-background shadow-sm overflow-hidden',
          hasUnsavedChanges && 'border-amber-500/50 border-dashed'
        )}
      >
        <div className="flex items-center gap-1 px-2 py-2">
          {/* Left: Status info */}
          <div className="flex items-center gap-1 flex-1">
            {plan && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="flex items-center gap-1.5 px-2 py-1 rounded-md text-xs text-muted-foreground hover:bg-muted/40 cursor-default">
                    {plan.created_by === 'agent' ? (
                      <IconRobot className="h-3.5 w-3.5" />
                    ) : (
                      <IconUser className="h-3.5 w-3.5" />
                    )}
                    <span className="capitalize">{plan.created_by}</span>
                  </div>
                </TooltipTrigger>
                <TooltipContent>Created by {plan.created_by}</TooltipContent>
              </Tooltip>
            )}
            {hasUnsavedChanges ? (
              <span className="text-xs text-amber-500 px-2">
                Unsaved changes <span className="text-amber-500/60">· ⌘S to save</span>
              </span>
            ) : plan?.updated_at ? (
              <span className="text-xs text-muted-foreground/60 px-2">
                Saved {formatRelativeTime(plan.updated_at)}
              </span>
            ) : null}
          </div>

          {/* Right: Actions */}
          <div className="flex items-center gap-0.5 shrink-0">
            {hasUnsavedChanges && (
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-muted-foreground hover:text-destructive"
                      onClick={onDiscard}
                      disabled={isSaving}
                    >
                      <IconX className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Discard changes</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40"
                      onClick={onSave}
                      disabled={isSaving}
                    >
                      {isSaving ? (
                        <IconLoader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <IconDeviceFloppy className="h-4 w-4" />
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Save plan</TooltipContent>
                </Tooltip>
              </>
            )}

            {/* Separator before utility actions */}
            {(hasUnsavedChanges || plan || hasDraftContent) && (
              <div className="h-4 w-px bg-border mx-1" />
            )}

            {/* Copy to clipboard */}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-muted-foreground"
                  onClick={onCopy}
                  disabled={!hasDraftContent}
                >
                  <IconCopy className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Copy to clipboard</TooltipContent>
            </Tooltip>

            {/* Re-analyze with agent */}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-muted-foreground"
                  onClick={onReanalyze}
                  disabled={isAgentBusy || !hasActiveSession || isReanalyzing}
                >
                  {isReanalyzing ? (
                    <IconLoader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <IconRefresh className="h-4 w-4" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>Ask agent to review plan</TooltipContent>
            </Tooltip>

            {/* Clear/Delete plan */}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40 text-muted-foreground hover:text-destructive"
                  onClick={onClear}
                  disabled={!plan && !hasDraftContent}
                >
                  <IconTrash className="h-4 w-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Clear plan</TooltipContent>
            </Tooltip>

            {/* Separator before comments */}
            {hasComments && <div className="h-4 w-px bg-border mx-1" />}

            {/* Comments section */}
            {hasComments && (
              <div className="flex items-center gap-1">
                {/* Clear comments */}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-7 w-7 p-0 cursor-pointer hover:bg-muted/40 text-muted-foreground hover:text-destructive"
                      onClick={onClearComments}
                      disabled={isSubmittingComments}
                    >
                      <IconX className="h-3.5 w-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Clear all comments</TooltipContent>
                </Tooltip>

                {/* Submit comments button - round with count inside */}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="icon"
                      className="h-7 w-7 rounded-full cursor-pointer bg-primary hover:bg-primary/90 text-primary-foreground relative"
                      onClick={onSubmitComments}
                      disabled={isSubmittingComments || isAgentBusy}
                    >
                      {isSubmittingComments ? (
                        <IconLoader2 className="h-4 w-4 animate-spin" />
                      ) : (
                        <>
                          <IconSend className="h-3.5 w-3.5" />
                          <span className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-primary-foreground text-primary text-[10px] font-semibold">
                            {commentCount}
                          </span>
                        </>
                      )}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Send {commentCount} comment{commentCount !== 1 ? 's' : ''} to agent</TooltipContent>
                </Tooltip>
              </div>
            )}

            {/* Separator before approve button */}
            {showApproveButton && onApprove && (
              <div className="h-4 w-px bg-border mx-1" />
            )}

            {/* Approve button - styled like submit button */}
            {showApproveButton && onApprove && (
              <div className="ml-1">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="icon"
                      className="h-7 w-7 rounded-full cursor-pointer bg-emerald-500 hover:bg-emerald-600 text-white"
                      onClick={onApprove}
                    >
                      <IconArrowRight className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Approve plan and continue</TooltipContent>
                </Tooltip>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
});

