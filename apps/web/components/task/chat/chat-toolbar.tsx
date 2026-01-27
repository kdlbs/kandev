'use client';

import { IconBrain, IconCheck, IconListCheck, IconLoader2, IconPaperclip } from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { KeyboardShortcutTooltip } from '@/components/keyboard-shortcut-tooltip';
import { TokenUsageDisplay } from '@/components/task/chat/token-usage-display';
import { SessionsDropdown } from '@/components/task/sessions-dropdown';
import { ModelSelector } from '@/components/task/model-selector';

type ChatToolbarProps = {
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  taskDescription: string;
  planModeEnabled: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  isAgentBusy: boolean;
  isStarting: boolean;
  isSending: boolean;
  onCancel: () => void;
  showApproveButton?: boolean;
  onApprove?: () => void;
};

export function ChatToolbar({
  sessionId,
  taskId,
  taskTitle,
  taskDescription,
  planModeEnabled,
  onPlanModeChange,
  isAgentBusy,
  isStarting,
  isSending,
  onCancel,
  showApproveButton,
  onApprove,
}: ChatToolbarProps) {
  return (
    <div className="flex items-center justify-between gap-2">
      <div className="flex items-center gap-2">
        <ModelSelector sessionId={sessionId} />
        <DropdownMenu>
          <Tooltip>
            <TooltipTrigger asChild>
              <DropdownMenuTrigger asChild>
                <Button type="button" variant="outline" size="icon" className="h-7 w-7 cursor-pointer">
                  <IconBrain className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
            </TooltipTrigger>
            <TooltipContent>Thinking level</TooltipContent>
          </Tooltip>
          <DropdownMenuContent align="start" side="top">
            <DropdownMenuItem>High</DropdownMenuItem>
            <DropdownMenuItem>Medium</DropdownMenuItem>
            <DropdownMenuItem>Low</DropdownMenuItem>
            <DropdownMenuItem>Off</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="icon"
                className={cn(
                  'h-7 w-7 cursor-pointer',
                  planModeEnabled &&
                    'bg-primary/15 text-primary border-primary/40 shadow-[0_0_0_1px_rgba(59,130,246,0.35)]'
                )}
                onClick={() => onPlanModeChange(!planModeEnabled)}
              >
                <IconListCheck className="h-4 w-4" />
              </Button>
              {planModeEnabled && <span className="text-xs font-medium text-primary">Plan mode active</span>}
            </div>
          </TooltipTrigger>
          <TooltipContent>Toggle plan mode</TooltipContent>
        </Tooltip>
        {/* Context window usage indicator */}
        <TokenUsageDisplay sessionId={sessionId} />
      </div>
      <div className="flex items-center gap-2">
        <SessionsDropdown
          taskId={taskId}
          activeSessionId={sessionId}
          taskTitle={taskTitle}
          taskDescription={taskDescription}
        />
        <Tooltip>
          <TooltipTrigger asChild>
            <Button type="button" variant="outline" size="icon" className="h-7 w-7 cursor-pointer">
              <IconPaperclip className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Add attachments</TooltipContent>
        </Tooltip>
        {showApproveButton && onApprove && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="outline"
                className="h-7 gap-1 border-green-300 text-green-700 hover:bg-green-50 hover:text-green-800"
                onClick={onApprove}
              >
                <IconCheck className="h-3.5 w-3.5" />
                Approve
              </Button>
            </TooltipTrigger>
            <TooltipContent>Approve and move to next step</TooltipContent>
          </Tooltip>
        )}
        <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT} enabled={!isAgentBusy && !isStarting}>
          <Button
            type={isAgentBusy ? 'button' : 'submit'}
            variant={isAgentBusy ? 'destructive' : 'default'}
            className={cn('h-7', isAgentBusy && 'gap-2')}
            disabled={isStarting || isSending}
            onClick={isAgentBusy ? onCancel : undefined}
          >
            {isAgentBusy ? (
              <>
                <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
                Stop
              </>
            ) : (
              'Submit'
            )}
          </Button>
        </KeyboardShortcutTooltip>
      </div>
    </div>
  );
}
