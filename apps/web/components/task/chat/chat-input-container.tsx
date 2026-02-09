'use client';

import { useRef, useCallback, useState, useEffect, memo, forwardRef, useImperativeHandle, type KeyboardEvent as ReactKeyboardEvent } from 'react';
import {
  IconArrowUp,
  IconAlertTriangle,
  IconFileTextSpark,
  IconPlus,
  IconPlayerPauseFilled,
  IconMessageDots,
  IconX,
} from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { SHORTCUTS } from '@/lib/keyboard/constants';
import { KeyboardShortcutTooltip } from '@/components/keyboard-shortcut-tooltip';
import { TokenUsageDisplay } from '@/components/task/chat/token-usage-display';
import { SessionsDropdown } from '@/components/task/sessions-dropdown';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { ModelSelector } from '@/components/task/model-selector';
import { RichTextInput, type RichTextInputHandle } from './rich-text-input';
import { MentionMenu } from './mention-menu';
import { SlashCommandMenu } from './slash-command-menu';
import { ClarificationInputOverlay } from './clarification-input-overlay';
import { CommentBlocksContainer } from './comment-block';
import { ChatInputFocusHint } from './chat-input-focus-hint';
import { ResizeHandle } from './resize-handle';
import { TodoSummary } from './todo-summary';
import { DocumentReferenceIndicator } from './document-reference-indicator';
import { QueuedMessageIndicator, type QueuedMessageIndicatorHandle } from './queued-message-indicator';
import { useResizableInput } from '@/hooks/use-resizable-input';
import {
  ImageAttachmentPreview,
  processImageFile,
  MAX_IMAGES,
  MAX_TOTAL_SIZE,
  type ImageAttachment,
} from './image-attachment-preview';
import { useInlineMention } from '@/hooks/use-inline-mention';
import { useInlineSlash } from '@/hooks/use-inline-slash';
import type { Message } from '@/lib/types/http';
import type { DiffComment } from '@/lib/diff/types';
import type { ActiveDocument } from '@/lib/state/slices/ui/types';

// Re-export ImageAttachment type for consumers
export type { ImageAttachment } from './image-attachment-preview';

// Type for message attachments sent to backend
export type MessageAttachment = {
  type: 'image';
  data: string;      // Base64 data
  mime_type: string; // MIME type
};

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
    <div className="flex items-center gap-1 px-1 pt-0 pb-0.5 border-t border-border">
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
              <IconFileTextSpark className="h-4 w-4" />
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
                onClick={onCancel}
              >
                <IconPlayerPauseFilled className="h-3.5 w-3.5" />
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

type TodoItem = {
  text: string;
  done?: boolean;
};

type ChatInputContainerProps = {
  onSubmit: (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[]) => void;
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
  /** Whether the session is in a terminal state (FAILED/CANCELLED) */
  isFailed?: boolean;
  /** Whether a message is queued */
  isQueued?: boolean;
  /** Callback to start editing the queued message (from keyboard navigation) */
  onStartQueueEdit?: () => void;
  /** User message history for up/down arrow navigation */
  userMessageHistory?: string[];
  /** Plan/document comments that will be included with the next message */
  documentCommentCount?: number;
  /** Callback to clear plan/document comments */
  onClearDocumentComments?: () => void;
  /** Todo items from processed messages */
  todoItems?: TodoItem[];
  /** Active document reference */
  activeDocument?: ActiveDocument | null;
  /** Queued message content */
  queuedMessage?: string | null;
  /** Cancel queued message */
  onCancelQueue?: () => void;
  /** Update queued message content */
  updateQueueContent?: (content: string) => Promise<void>;
  /** Ref for queued message indicator */
  queuedMessageRef?: React.RefObject<QueuedMessageIndicatorHandle | null>;
  /** Callback when queue edit completes */
  onQueueEditComplete?: () => void;
  /** Whether the dockview group containing this panel is focused */
  isPanelFocused?: boolean;
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
      isFailed = false,
      isQueued = false,
      onStartQueueEdit,
      userMessageHistory = [],
      documentCommentCount = 0,
      onClearDocumentComments,
      todoItems = [],
      activeDocument,
      queuedMessage,
      onCancelQueue,
      updateQueueContent,
      queuedMessageRef,
      onQueueEditComplete,
      isPanelFocused,
    },
    ref
  ) {
    // Keep input state local to avoid re-rendering parent on every keystroke
    const [value, setValue] = useState('');
    const [isInputFocused, setIsInputFocused] = useState(false);
    const [attachments, setAttachments] = useState<ImageAttachment[]>([]);
    const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
    const inputRef = useRef<RichTextInputHandle>(null);

    // Message history navigation state
    const [historyIndex, setHistoryIndex] = useState(-1); // -1 means not navigating history
    const [historyBuffer, setHistoryBuffer] = useState(''); // Store current input when navigating history

    // Resizable input
    const { height, containerRef, resizeHandleProps } = useResizableInput();

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

    // Ref for attachments to keep handleSubmit stable
    const attachmentsRef = useRef(attachments);
    useEffect(() => {
      attachmentsRef.current = attachments;
    }, [attachments]);

    // Handle image paste
    const handlePaste = useCallback(async (e: ClipboardEvent) => {
      const items = e.clipboardData?.items;
      if (!items) return;

      // Check for image items
      const imageItems: DataTransferItem[] = [];
      for (const item of items) {
        if (item.type.startsWith('image/')) {
          imageItems.push(item);
        }
      }

      if (imageItems.length === 0) return;

      // Prevent default paste behavior for images
      e.preventDefault();

      // Check if we've hit the max images limit
      if (attachments.length >= MAX_IMAGES) {
        console.warn(`Maximum ${MAX_IMAGES} images allowed`);
        return;
      }

      // Calculate current total size
      const currentTotalSize = attachments.reduce((sum, att) => sum + att.size, 0);

      // Process each image
      for (const item of imageItems) {
        if (attachments.length + imageItems.indexOf(item) >= MAX_IMAGES) break;

        const file = item.getAsFile();
        if (!file) continue;

        // Check total size limit
        if (currentTotalSize + file.size > MAX_TOTAL_SIZE) {
          console.warn('Total attachment size limit exceeded');
          break;
        }

        const attachment = await processImageFile(file);
        if (attachment) {
          setAttachments(prev => [...prev, attachment]);
        }
      }
    }, [attachments]);

    // Attach paste listener to the container
    useEffect(() => {
      const textarea = inputRef.current?.getTextareaElement();
      if (!textarea) return;

      textarea.addEventListener('paste', handlePaste);
      return () => {
        textarea.removeEventListener('paste', handlePaste);
      };
    }, [handlePaste]);

    // Remove attachment handler
    const handleRemoveAttachment = useCallback((id: string) => {
      setAttachments(prev => prev.filter(att => att.id !== id));
    }, []);

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

        // Reset history navigation when user modifies content from history (but not when editing queued message)
        if (historyIndex >= 0) {
          // User is modifying a historical message - keep the content but exit history mode
          setHistoryIndex(-1);
          setHistoryBuffer('');
        }

        // Dismiss the request changes tooltip when user starts typing
        if (showRequestChangesTooltip && onRequestChangesTooltipDismiss) {
          onRequestChangesTooltipDismiss();
        }
      },
      [mention, slash, showRequestChangesTooltip, onRequestChangesTooltipDismiss, historyIndex]
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

        // Handle up/down arrow for history navigation
        const textarea = event.currentTarget;
        const cursorPosition = textarea.selectionStart;
        const textLength = textarea.value.length;

        // Up arrow: navigate to previous message or start editing queued message
        if (event.key === 'ArrowUp' && cursorPosition === 0) {
          event.preventDefault();

          // If there's a queued message and we're not navigating history, trigger edit mode on QueuedMessageIndicator
          if (isQueued && historyIndex === -1 && onStartQueueEdit) {
            onStartQueueEdit();
            return;
          }

          // Navigate to previous message in history
          if (userMessageHistory.length > 0) {
            const newIndex = historyIndex === -1
              ? userMessageHistory.length - 1
              : Math.max(0, historyIndex - 1);

            if (historyIndex === -1) {
              setHistoryBuffer(value);
            }

            setValue(userMessageHistory[newIndex] || '');
            setHistoryIndex(newIndex);
          }
        }
        // Down arrow: navigate to next message in history
        else if (event.key === 'ArrowDown' && cursorPosition === textLength) {
          event.preventDefault();

          if (historyIndex >= 0) {
            const newIndex = historyIndex + 1;

            if (newIndex >= userMessageHistory.length) {
              // Restore the buffer (what user was typing before navigating)
              setValue(historyBuffer);
              setHistoryIndex(-1);
            } else {
              setValue(userMessageHistory[newIndex] || '');
              setHistoryIndex(newIndex);
            }
          }
        }
      },
      [mention, slash, userMessageHistory, historyIndex, historyBuffer, value, isQueued, onStartQueueEdit]
    );

    // Stable submit handler using refs - doesn't change on value/menu state changes
    const handleSubmit = useCallback(() => {
      // Don't submit if already sending
      if (isSending) return;

      // Don't submit if a menu is open
      if (menuStateRef.current.mentionOpen || menuStateRef.current.slashOpen) return;

      const trimmed = valueRef.current.trim();
      const comments = pendingCommentsRef.current;
      const currentAttachments = attachmentsRef.current;

      // Collect all pending comments
      const allComments: DiffComment[] = [];
      if (comments) {
        for (const filePath of Object.keys(comments)) {
          allComments.push(...comments[filePath]);
        }
      }

      // Allow submission if there's text OR comments OR attachments
      if (!trimmed && allComments.length === 0 && currentAttachments.length === 0) return;

      // Convert attachments to MessageAttachment format for backend
      const messageAttachments: MessageAttachment[] = currentAttachments.map(att => ({
        type: 'image' as const,
        data: att.data,
        mime_type: att.mimeType,
      }));

      // Submit message - handler will queue it if agent is busy
      onSubmit(
        trimmed,
        allComments.length > 0 ? allComments : undefined,
        messageAttachments.length > 0 ? messageAttachments : undefined
      );
      setValue('');
      setAttachments([]); // Clear attachments after submit
      setHistoryIndex(-1);
      setHistoryBuffer('');
    }, [onSubmit, isSending]);

    // Disable input when starting, sending, or session ended (failed/cancelled)
    // Note: We allow typing when agent is busy so users can queue messages
    const isDisabled = isStarting || isSending || isFailed;
    const hasPendingClarification = pendingClarification && onClarificationResolved;
    const hasPendingComments = pendingCommentsByFile && Object.keys(pendingCommentsByFile).length > 0;

    // Dynamic placeholder
    const inputPlaceholder =
      placeholder ||
      (isAgentBusy
        ? 'Queue more instructions...'
        : hasAgentCommands
          ? 'Ask to make changes, @mention files, run /commands'
          : 'Ask to make changes, @mention files');

    // Show focus hint when input not focused, no clarification, and no comments
    const showFocusHint = !isInputFocused && !hasPendingClarification && !hasPendingComments;

    // Determine if we have any context items to show
    const hasQueuedMessage = isQueued && queuedMessage && onCancelQueue && updateQueueContent;
    const hasTodos = todoItems.length > 0;
    const hasContextItems =
      hasQueuedMessage ||
      hasTodos ||
      !!activeDocument ||
      hasPendingComments ||
      documentCommentCount > 0 ||
      attachments.length > 0 ||
      hasPendingClarification;

    if (isFailed) {
      return (
        <>
          <div className="rounded border border-border overflow-hidden">
            <div className="flex items-center justify-between gap-3 px-4 py-3">
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <IconAlertTriangle className="h-4 w-4 text-orange-500 shrink-0" />
                <span>This session has ended. Start a new session to continue.</span>
              </div>
              <Button
                variant="outline"
                size="sm"
                className="shrink-0 gap-1.5 cursor-pointer"
                onClick={() => setShowNewSessionDialog(true)}
              >
                <IconPlus className="h-3.5 w-3.5" />
                New Session
              </Button>
            </div>
          </div>
          <TaskCreateDialog
            open={showNewSessionDialog}
            onOpenChange={setShowNewSessionDialog}
            mode="session"
            workspaceId={null}
            boardId={null}
            defaultColumnId={null}
            columns={[]}
            taskId={taskId}
            initialValues={{
              title: taskTitle ?? '',
              description: taskDescription,
            }}
          />
        </>
      );
    }

    return (
      <div
        ref={containerRef}
        className={cn(
          'relative flex flex-col border rounded ',
          isPanelFocused ? 'bg-background border-border' : 'bg-background/40 border-border',
          isAgentBusy && 'chat-input-running',
          hasPendingClarification && 'border-blue-500/50',
          showRequestChangesTooltip && 'animate-pulse border-orange-500',
          hasPendingComments && 'border-amber-500/50',
        )}
        style={{ height }}
      >
        {/* Resize handle */}
        <ResizeHandle visible={isPanelFocused || isInputFocused} {...resizeHandleProps} />

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

        {/* CONTEXT ZONE: scrollable, above editor */}
        {hasContextItems && (
          <div className="overflow-y-auto max-h-[40%] border-b border-border/50 shrink-0">
            <div className="px-2 pt-2 pb-1 space-y-1.5">
              {/* Queued message */}
              {hasQueuedMessage && (
                <QueuedMessageIndicator
                  ref={queuedMessageRef}
                  content={queuedMessage}
                  onCancel={onCancelQueue}
                  onUpdate={updateQueueContent}
                  isVisible={true}
                  onEditComplete={onQueueEditComplete}
                />
              )}

              {/* Todo summary */}
              {hasTodos && <TodoSummary todos={todoItems} />}

              {/* Document reference */}
              {activeDocument && (
                <DocumentReferenceIndicator activeDocument={activeDocument} />
              )}

              {/* Pending comment blocks */}
              {hasPendingComments && sessionId && onRemoveCommentFile && onRemoveComment && (
                <CommentBlocksContainer
                  commentsByFile={pendingCommentsByFile}
                  sessionId={sessionId}
                  onRemoveFile={onRemoveCommentFile}
                  onRemoveComment={onRemoveComment}
                  onCommentClick={onCommentClick}
                />
              )}

              {/* Plan comments indicator */}
              {documentCommentCount > 0 && (
                <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/30 px-2 py-1.5">
                  <IconMessageDots className="h-3.5 w-3.5 text-purple-500 shrink-0" />
                  <span className="text-xs text-muted-foreground flex-1">
                    {documentCommentCount} plan comment{documentCommentCount !== 1 ? 's' : ''} will be included
                  </span>
                  {onClearDocumentComments && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={onClearDocumentComments}
                      className="h-5 w-5 cursor-pointer p-0 hover:text-destructive"
                    >
                      <IconX className="h-3 w-3" />
                    </Button>
                  )}
                </div>
              )}

              {/* Image attachments preview */}
              <ImageAttachmentPreview
                attachments={attachments}
                onRemove={handleRemoveAttachment}
                disabled={isDisabled}
              />

              {/* Clarification (renders above editor, textarea stays visible but disabled) */}
              {hasPendingClarification && (
                <div className="overflow-auto">
                  <ClarificationInputOverlay
                    message={pendingClarification}
                    onResolved={onClarificationResolved}
                  />
                </div>
              )}
            </div>
          </div>
        )}

        {/* EDITOR ZONE: flex-1, fills remaining space */}
        <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
          <Tooltip open={showRequestChangesTooltip}>
            <TooltipTrigger asChild>
              <div className="flex-1 min-h-0">
                <RichTextInput
                  ref={inputRef}
                  value={value}
                  onChange={handleChange}
                  onKeyDown={handleKeyDown}
                  onSubmit={handleSubmit}
                  placeholder={inputPlaceholder}
                  disabled={isDisabled || !!hasPendingClarification}
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
      </div>
    );
  });
