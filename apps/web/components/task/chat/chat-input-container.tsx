'use client';

import { useRef, useCallback, useState, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  IconArrowUp,
  IconBrain,
  IconCheck,
  IconListCheck,
  IconLoader2,
  IconPlayerStopFilled,
} from '@tabler/icons-react';
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
import { RichTextInput, type RichTextInputHandle } from './rich-text-input';
import { MentionMenu } from './mention-menu';
import { SlashCommandMenu } from './slash-command-menu';
import { ClarificationInputOverlay } from './clarification-input-overlay';
import { useInlineMention } from '@/hooks/use-inline-mention';
import { useInlineSlash } from '@/hooks/use-inline-slash';
import type { Message } from '@/lib/types/http';

type ChatInputContainerProps = {
  onSubmit: (message: string) => void;
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
  placeholder?: string;
  pendingClarification?: Message | null;
  onClarificationResolved?: () => void;
};

export function ChatInputContainer({
  onSubmit,
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
  placeholder,
  pendingClarification,
  onClarificationResolved,
}: ChatInputContainerProps) {
  // Keep input state local to avoid re-rendering parent on every keystroke
  const [value, setValue] = useState('');
  const inputRef = useRef<RichTextInputHandle>(null);

  // Mention menu hook
  const mention = useInlineMention(inputRef, value, setValue, sessionId);

  // Slash command menu hook
  const slash = useInlineSlash(inputRef, value, setValue, onPlanModeChange);

  // Combined change handler
  const handleChange = useCallback(
    (newValue: string) => {
      mention.handleChange(newValue);
      slash.handleChange(newValue);
    },
    [mention, slash]
  );

  // Combined keydown handler
  const handleKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
      // Let menus handle navigation first
      if (mention.isOpen) {
        mention.handleKeyDown(event);
        if (event.defaultPrevented) return;
      }
      if (slash.isOpen) {
        slash.handleKeyDown(event);
        if (event.defaultPrevented) return;
      }
    },
    [mention, slash]
  );

  // Handle form submission
  const handleSubmit = useCallback(() => {
    // Don't submit if a menu is open
    if (mention.isOpen || slash.isOpen) return;
    const trimmed = value.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setValue('');
  }, [mention.isOpen, slash.isOpen, value, onSubmit]);

  const isDisabled = isStarting || isSending;
  const hasPendingClarification = pendingClarification && onClarificationResolved;

  return (
    <div
      className={cn(
        'rounded-2xl border border-border bg-background shadow-md overflow-hidden',
        planModeEnabled && 'border-primary border-dashed',
        hasPendingClarification && 'border-blue-500/50'
      )}
    >
      {/* Popup menus */}
      <MentionMenu
        isOpen={mention.isOpen}
        isLoading={mention.isLoading}
        position={mention.position}
        items={mention.items}
        query={mention.query}
        selectedIndex={mention.selectedIndex}
        onSelect={mention.handleSelect}
        onClose={mention.closeMenu}
        setSelectedIndex={mention.setSelectedIndex}
      />
      <SlashCommandMenu
        isOpen={slash.isOpen}
        position={slash.position}
        commands={slash.commands}
        selectedIndex={slash.selectedIndex}
        onSelect={slash.handleSelect}
        onClose={slash.closeMenu}
        setSelectedIndex={slash.setSelectedIndex}
      />

      {/* Clarification inline (replaces input area when pending) */}
      {hasPendingClarification ? (
        <div style={{ height: '88px', overflow: 'auto' }}>
          <ClarificationInputOverlay
            message={pendingClarification}
            onResolved={onClarificationResolved}
          />
        </div>
      ) : (
        /* Normal input area */
        <RichTextInput
          ref={inputRef}
          value={value}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onSubmit={handleSubmit}
          placeholder={placeholder}
          disabled={isDisabled}
          planModeEnabled={planModeEnabled}
        />
      )}

      {/* Integrated toolbar */}
      <div className="flex items-center gap-1 px-2 py-2 border-t border-border">
        {/* Left: Plan, Thinking, Context */}
        <div className="flex items-center gap-0.5">
          {/* Plan mode toggle */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className={cn(
                  'h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40',
                  planModeEnabled && 'bg-primary/15 text-primary'
                )}
                onClick={() => onPlanModeChange(!planModeEnabled)}
              >
                <IconListCheck className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Toggle plan mode</TooltipContent>
          </Tooltip>

          {/* Thinking level dropdown */}
          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 gap-1.5 px-2 cursor-pointer hover:bg-muted/40"
                  >
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

        </div>

        {/* Spacer */}
        <div className="flex-1" />

        {/* Right: Sessions, Model, Submit */}
        <div className="flex items-center gap-0.5 shrink-0">
          <SessionsDropdown
            taskId={taskId}
            activeSessionId={sessionId}
            taskTitle={taskTitle}
            taskDescription={taskDescription}
          />

          {/* Token usage display */}
          <TokenUsageDisplay sessionId={sessionId} />

          <ModelSelector sessionId={sessionId} />

          {/* Approve button */}
          {showApproveButton && onApprove && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  size="sm"
                  className="h-7 cursor-pointer gap-1.5 px-3 bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/25 hover:border-emerald-500/50"
                  onClick={onApprove}
                >
                  <IconCheck className="h-3.5 w-3.5" />
                  <span className="font-medium">Approve</span>
                </Button>
              </TooltipTrigger>
              <TooltipContent>Approve and move to next step</TooltipContent>
            </Tooltip>
          )}

          {/* Submit/Stop button */}
          <div className="ml-1">
            <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT} enabled={!isAgentBusy && !isStarting}>
              {isAgentBusy ? (
                <Button
                  type="button"
                  variant="secondary"
                  size="icon"
                  className="h-7 w-7 rounded-full cursor-pointer bg-destructive/10 text-destructive hover:bg-destructive/20"
                  disabled={isDisabled}
                  onClick={onCancel}
                >
                  <IconPlayerStopFilled className="h-3.5 w-3.5" />
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="default"
                  size="icon"
                  className="h-7 w-7 rounded-full cursor-pointer"
                  disabled={isDisabled}
                  onClick={handleSubmit}
                >
                  {isSending ? (
                    <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <IconArrowUp className="h-4 w-4" />
                  )}
                </Button>
              )}
            </KeyboardShortcutTooltip>
          </div>
        </div>
      </div>
    </div>
  );
}
