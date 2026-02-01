'use client';

import { useRef, useCallback, useState, useEffect, memo, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  IconArrowUp,
  IconBrain,
  IconListCheck,
  IconPlayerStopFilled,
} from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
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

// Memoized toolbar to prevent re-renders on input value changes
type ChatInputToolbarProps = {
  planModeEnabled: boolean;
  onPlanModeChange: (enabled: boolean) => void;
  sessionId: string | null;
  taskId: string | null;
  taskTitle?: string;
  taskDescription: string;
  isAgentBusy: boolean;
  isDisabled: boolean;
  isSending: boolean;
  onCancel: () => void;
  onSubmit: () => void;
};

const ChatInputToolbar = memo(function ChatInputToolbar({
  planModeEnabled,
  onPlanModeChange,
  sessionId,
  taskId,
  taskTitle,
  taskDescription,
  isAgentBusy,
  isDisabled,
  isSending,
  onCancel,
  onSubmit,
}: ChatInputToolbarProps) {
  return (
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

        {/* Submit/Stop button */}
        <div className="ml-1">
          <KeyboardShortcutTooltip shortcut={SHORTCUTS.SUBMIT} enabled={!isAgentBusy && !isDisabled}>
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
                onClick={onSubmit}
              >
                {isSending ? (
                  <GridSpinner className="text-primary-foreground" />
                ) : (
                  <IconArrowUp className="h-4 w-4" />
                )}
              </Button>
            )}
          </KeyboardShortcutTooltip>
        </div>
      </div>
    </div>
  );
});

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
  placeholder?: string;
  pendingClarification?: Message | null;
  onClarificationResolved?: () => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
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
  placeholder,
  pendingClarification,
  onClarificationResolved,
  showRequestChangesTooltip = false,
}: ChatInputContainerProps) {
  // Keep input state local to avoid re-rendering parent on every keystroke
  const [value, setValue] = useState('');
  const inputRef = useRef<RichTextInputHandle>(null);

  // Focus input when request changes tooltip is shown
  useEffect(() => {
    if (showRequestChangesTooltip && inputRef.current) {
      inputRef.current.focus();
    }
  }, [showRequestChangesTooltip]);

  // Use ref for value to keep handleSubmit stable (doesn't change on every keystroke)
  const valueRef = useRef(value);
  useEffect(() => {
    valueRef.current = value;
  }, [value]);

  // Mention menu hook
  const mention = useInlineMention(inputRef, value, setValue, sessionId);

  // Handler for agent slash commands - sends /{commandName} as a message
  const handleAgentCommand = useCallback(
    (commandName: string) => {
      onSubmit(`/${commandName}`);
    },
    [onSubmit]
  );

  // Slash command menu hook
  const slash = useInlineSlash(inputRef, value, setValue, {
    sessionId,
    onAgentCommand: handleAgentCommand,
  });

  // Use refs for menu state to keep handleSubmit stable
  const menuStateRef = useRef({ mentionOpen: mention.isOpen, slashOpen: slash.isOpen });
  useEffect(() => {
    menuStateRef.current = { mentionOpen: mention.isOpen, slashOpen: slash.isOpen };
  }, [mention.isOpen, slash.isOpen]);

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

  // Stable submit handler using refs - doesn't change on value/menu state changes
  const handleSubmit = useCallback(() => {
    // Don't submit if a menu is open
    if (menuStateRef.current.mentionOpen || menuStateRef.current.slashOpen) return;
    const trimmed = valueRef.current.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setValue('');
  }, [onSubmit]);

  const isDisabled = isStarting || isSending;
  const hasPendingClarification = pendingClarification && onClarificationResolved;

  return (
    <div
      className={cn(
        'rounded-2xl border border-border bg-background shadow-md overflow-hidden',
        planModeEnabled && 'border-primary border-dashed',
        hasPendingClarification && 'border-blue-500/50',
        showRequestChangesTooltip && 'animate-pulse border-purple-500'
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
        <div style={{ overflow: 'auto' }}>
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

      {/* Integrated toolbar - memoized to prevent re-renders on input changes */}
      <ChatInputToolbar
        planModeEnabled={planModeEnabled}
        onPlanModeChange={onPlanModeChange}
        sessionId={sessionId}
        taskId={taskId}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
        isAgentBusy={isAgentBusy}
        isDisabled={isDisabled}
        isSending={isSending}
        onCancel={onCancel}
        onSubmit={handleSubmit}
      />
    </div>
  );
}
