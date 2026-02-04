'use client';

import { useRef, useCallback, useState, useEffect, memo, forwardRef, useImperativeHandle, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  IconArrowUp,
  IconListCheck,
  IconPlayerStopFilled,
} from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { Button } from '@kandev/ui/button';
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
import { CommentBlocksContainer } from './comment-block';
import { ChatInputFocusHint } from './chat-input-focus-hint';
import { useInlineMention } from '@/hooks/use-inline-mention';
import { useInlineSlash } from '@/hooks/use-inline-slash';
import type { Message } from '@/lib/types/http';
import type { DiffComment } from '@/lib/diff/types';

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
  submitKey?: 'enter' | 'cmd_enter';
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
  submitKey = 'cmd_enter',
}: ChatInputToolbarProps) {
  const submitShortcut = submitKey === 'enter' ? SHORTCUTS.SUBMIT_ENTER : SHORTCUTS.SUBMIT;

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
          <KeyboardShortcutTooltip shortcut={submitShortcut} enabled={!isAgentBusy && !isDisabled}>
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

export type ChatInputContainerHandle = {
  focusInput: () => void;
  getTextareaElement: () => HTMLTextAreaElement | null;
  getValue: () => string;
  getSelectionStart: () => number;
  insertText: (text: string, from: number, to: number) => void;
};

type ChatInputContainerProps = {
  onSubmit: (message: string, reviewComments?: DiffComment[]) => void;
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
  /** Pending review comments grouped by file */
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  /** Callback to remove comments for a file */
  onRemoveCommentFile?: (filePath: string) => void;
  /** Callback to remove a specific comment */
  onRemoveComment?: (sessionId: string, filePath: string, commentId: string) => void;
  /** Callback when a comment is clicked (for jump-to-line) */
  onCommentClick?: (comment: DiffComment) => void;
  /** Chat submit key preference */
  submitKey?: 'enter' | 'cmd_enter';
  /** Whether the current agent has commands (affects placeholder) */
  hasAgentCommands?: boolean;
};

export const ChatInputContainer = forwardRef<ChatInputContainerHandle, ChatInputContainerProps>(
  function ChatInputContainer(
    {
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
      onRequestChangesTooltipDismiss,
      pendingCommentsByFile,
      onRemoveCommentFile,
      onRemoveComment,
      onCommentClick,
      submitKey = 'cmd_enter',
      hasAgentCommands = false,
    },
    ref
  ) {
  // Keep input state local to avoid re-rendering parent on every keystroke
  const [value, setValue] = useState('');
  const [isInputFocused, setIsInputFocused] = useState(false);
  const inputRef = useRef<RichTextInputHandle>(null);

  // Expose imperative handle for parent
  useImperativeHandle(
    ref,
    () => ({
      focusInput: () => inputRef.current?.focus(),
      getTextareaElement: () => inputRef.current?.getTextareaElement() ?? null,
      getValue: () => inputRef.current?.getValue() ?? '',
      getSelectionStart: () => inputRef.current?.getSelectionStart() ?? 0,
      insertText: (text: string, from: number, to: number) => {
        inputRef.current?.insertText(text, from, to);
      },
    }),
    []
  );

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

  // Ref for pending comments
  const pendingCommentsRef = useRef(pendingCommentsByFile);
  useEffect(() => {
    pendingCommentsRef.current = pendingCommentsByFile;
  }, [pendingCommentsByFile]);

  // Combined change handler
  const handleChange = useCallback(
    (newValue: string) => {
      mention.handleChange(newValue);
      slash.handleChange(newValue);
      // Dismiss the request changes tooltip when user starts typing
      if (showRequestChangesTooltip && onRequestChangesTooltipDismiss) {
        onRequestChangesTooltipDismiss();
      }
    },
    [mention, slash, showRequestChangesTooltip, onRequestChangesTooltipDismiss]
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
    // Don't submit if agent is busy or already sending
    if (isAgentBusy || isSending) return;

    // Don't submit if a menu is open
    if (menuStateRef.current.mentionOpen || menuStateRef.current.slashOpen) return;

    const trimmed = valueRef.current.trim();
    const comments = pendingCommentsRef.current;

    // Collect all pending comments
    const allComments: DiffComment[] = [];
    if (comments) {
      for (const filePath of Object.keys(comments)) {
        allComments.push(...comments[filePath]);
      }
    }

    // Allow submission if there's text OR comments
    if (!trimmed && allComments.length === 0) return;

    onSubmit(trimmed, allComments.length > 0 ? allComments : undefined);
    setValue('');
  }, [onSubmit, isAgentBusy, isSending]);

  // Disable input when agent is busy (RUNNING state), starting, or sending a message
  const isDisabled = isAgentBusy || isStarting || isSending;
  const hasPendingClarification = pendingClarification && onClarificationResolved;
  const hasPendingComments = pendingCommentsByFile && Object.keys(pendingCommentsByFile).length > 0;

  // Dynamic placeholder
  const inputPlaceholder =
    placeholder ||
    (hasAgentCommands
      ? 'Ask to make changes, @mention files, run /commands'
      : 'Ask to make changes, @mention files');

  // Show focus hint when input not focused, no clarification, and no comments
  const showFocusHint = !isInputFocused && !hasPendingClarification && !hasPendingComments;

  return (
    <div
      className={cn(
        'relative rounded-2xl border border-border bg-background shadow-md overflow-hidden',
        planModeEnabled && 'border-primary border-dashed',
        hasPendingClarification && 'border-blue-500/50',
        showRequestChangesTooltip && 'animate-pulse border-orange-500',
        hasPendingComments && 'border-amber-500/50'
      )}
    >
      {/* Focus hint */}
      <ChatInputFocusHint visible={showFocusHint} />
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

      {/* Pending comment blocks */}
      {hasPendingComments && sessionId && onRemoveCommentFile && onRemoveComment && (
        <div className="px-3 pt-3">
          <CommentBlocksContainer
            commentsByFile={pendingCommentsByFile}
            sessionId={sessionId}
            onRemoveFile={onRemoveCommentFile}
            onRemoveComment={onRemoveComment}
            onCommentClick={onCommentClick}
          />
        </div>
      )}

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
        <Tooltip open={showRequestChangesTooltip}>
          <TooltipTrigger asChild>
            <div>
              <RichTextInput
                ref={inputRef}
                value={value}
                onChange={handleChange}
                onKeyDown={handleKeyDown}
                onSubmit={handleSubmit}
                placeholder={inputPlaceholder}
                disabled={isDisabled}
                planModeEnabled={planModeEnabled}
                submitKey={submitKey}
                onFocus={() => setIsInputFocused(true)}
                onBlur={() => setIsInputFocused(false)}
              />
            </div>
          </TooltipTrigger>
          <TooltipContent side="top" className="bg-orange-600 text-white border-orange-700">
            <p className="font-medium">Write your changes here</p>
          </TooltipContent>
        </Tooltip>
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
        submitKey={submitKey}
      />
    </div>
  );
});
