'use client';

import { useRef, useCallback, useState, useEffect, useMemo, forwardRef, useImperativeHandle } from 'react';
import {
  IconAlertTriangle,
  IconPlus,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { TipTapInput, type TipTapInputHandle } from './tiptap-input';
import { ClarificationInputOverlay } from './clarification-input-overlay';
import { ChatInputFocusHint } from './chat-input-focus-hint';
import { ResizeHandle } from './resize-handle';
import { TodoSummary } from './todo-summary';
import { QueuedMessageIndicator, type QueuedMessageIndicatorHandle } from './queued-message-indicator';
import { ChatInputToolbar } from './chat-input-toolbar';
import { ContextZone } from './context-items/context-zone';
import type { ContextItem, ImageContextItem } from '@/lib/types/context';
import type { ContextFile } from '@/lib/state/context-files-store';
import { useResizableInput } from '@/hooks/use-resizable-input';
import {
  processImageFile,
  formatBytes,
  MAX_IMAGES,
  MAX_TOTAL_SIZE,
  type ImageAttachment,
} from './image-attachment-preview';
import {
  getChatDraftText,
  setChatDraftText,
  getChatDraftAttachments,
  setChatDraftAttachments,
  setChatDraftContent,
  restoreAttachmentPreview,
} from '@/lib/local-storage';
import type { Message } from '@/lib/types/http';
import type { DiffComment } from '@/lib/diff/types';

// Re-export ImageAttachment type for consumers
export type { ImageAttachment } from './image-attachment-preview';

// Type for message attachments sent to backend
export type MessageAttachment = {
  type: 'image';
  data: string;      // Base64 data
  mime_type: string; // MIME type
};

export type ChatInputContainerHandle = {
  focusInput: () => void;
  getTextareaElement: () => HTMLElement | null;
  getValue: () => string;
  getSelectionStart: () => number;
  insertText: (text: string, from: number, to: number) => void;
};

type TodoItem = {
  text: string;
  done?: boolean;
};

type ChatInputContainerProps = {
  onSubmit: (message: string, reviewComments?: DiffComment[], attachments?: MessageAttachment[], inlineMentions?: ContextFile[]) => void;
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
  /** Pending review comments grouped by file — kept for submit collection */
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  /** Chat submit key preference */
  submitKey?: 'enter' | 'cmd_enter';
  /** Whether the current agent has commands (affects placeholder) */
  hasAgentCommands?: boolean;
  /** Whether the session is in a terminal state (FAILED/CANCELLED) */
  isFailed?: boolean;
  /** Whether a message is queued */
  isQueued?: boolean;
  /** Context items to display as chips above input */
  contextItems?: ContextItem[];
  /** Whether plan is selected as context in the popover */
  planContextEnabled?: boolean;
  /** Context files for popover */
  contextFiles?: ContextFile[];
  /** Toggle a context file in/out */
  onToggleContextFile?: (file: ContextFile) => void;
  /** Add a context file (for @mention) */
  onAddContextFile?: (file: ContextFile) => void;
  /** Todo items from processed messages */
  todoItems?: TodoItem[];
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
      submitKey = 'cmd_enter',
      hasAgentCommands = false,
      isFailed = false,
      isQueued = false,
      contextItems = [],
      planContextEnabled = false,
      contextFiles = [],
      onToggleContextFile,
      onAddContextFile,
      todoItems = [],
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
    const [value, setValue] = useState(() =>
      sessionId ? getChatDraftText(sessionId) : ''
    );
    const [isInputFocused, setIsInputFocused] = useState(false);
    const [attachments, setAttachments] = useState<ImageAttachment[]>(() =>
      sessionId ? getChatDraftAttachments(sessionId).map(restoreAttachmentPreview) : []
    );
    const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
    const [contextPopoverOpen, setContextPopoverOpen] = useState(false);
    const inputRef = useRef<TipTapInputHandle>(null);

    // Message history navigation state
    const [historyIndex, setHistoryIndex] = useState(-1);

    // Hydrate draft when switching sessions (component stays mounted)
    const prevSessionIdRef = useRef(sessionId);
    useEffect(() => {
      if (sessionId === prevSessionIdRef.current) return;
      prevSessionIdRef.current = sessionId;
      /* eslint-disable react-hooks/set-state-in-effect -- syncing from localStorage on session switch */
      if (sessionId) {
        setValue(getChatDraftText(sessionId));
        setAttachments(getChatDraftAttachments(sessionId).map(restoreAttachmentPreview));
      } else {
        setValue('');
        setAttachments([]);
      }
      /* eslint-enable react-hooks/set-state-in-effect */
    }, [sessionId]);

    // Resizable input
    const { height, resetHeight, containerRef, resizeHandleProps } = useResizableInput(sessionId ?? undefined);

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

    // Use ref for value to keep handleSubmit stable
    const valueRef = useRef(value);
    useEffect(() => {
      valueRef.current = value;
    }, [value]);

    // Ref for attachments to keep handleSubmit stable
    const attachmentsRef = useRef(attachments);
    useEffect(() => {
      attachmentsRef.current = attachments;
      // Persist attachments when they change (skip during session switch — handled by hydrate effect)
      if (sessionId && prevSessionIdRef.current === sessionId) {
        setChatDraftAttachments(sessionId, attachments);
      }
    }, [attachments, sessionId]);

    // Handle image paste (called by TipTapInput's handlePaste editorProp)
    const handleImagePaste = useCallback(async (files: File[]) => {
      if (attachments.length >= MAX_IMAGES) {
        console.warn(`Maximum ${MAX_IMAGES} images allowed`);
        return;
      }

      const currentTotalSize = attachments.reduce((sum, att) => sum + att.size, 0);

      for (const file of files) {
        if (attachments.length >= MAX_IMAGES) break;

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

    // Remove attachment handler
    const handleRemoveAttachment = useCallback((id: string) => {
      setAttachments(prev => prev.filter(att => att.id !== id));
    }, []);

    // Handler for agent slash commands
    const handleAgentCommand = useCallback(
      (commandName: string) => {
        onSubmit(`/${commandName}`);
      },
      [onSubmit]
    );

    // Ref for pending comments
    const pendingCommentsRef = useRef(pendingCommentsByFile);
    useEffect(() => {
      pendingCommentsRef.current = pendingCommentsByFile;
    }, [pendingCommentsByFile]);

    // Change handler
    const handleChange = useCallback(
      (newValue: string) => {
        setValue(newValue);
        if (sessionId) setChatDraftText(sessionId, newValue);

        if (historyIndex >= 0) {
          setHistoryIndex(-1);
        }

        if (showRequestChangesTooltip && onRequestChangesTooltipDismiss) {
          onRequestChangesTooltipDismiss();
        }
      },
      [showRequestChangesTooltip, onRequestChangesTooltipDismiss, historyIndex, sessionId]
    );

    // Stable submit handler using refs
    const handleSubmit = useCallback(() => {
      if (isSending) return;

      const trimmed = valueRef.current.trim();
      const comments = pendingCommentsRef.current;
      const currentAttachments = attachmentsRef.current;

      const allComments: DiffComment[] = [];
      if (comments) {
        for (const filePath of Object.keys(comments)) {
          allComments.push(...comments[filePath]);
        }
      }

      if (!trimmed && allComments.length === 0 && currentAttachments.length === 0) return;

      const messageAttachments: MessageAttachment[] = currentAttachments.map(att => ({
        type: 'image' as const,
        data: att.data,
        mime_type: att.mimeType,
      }));

      // Extract inline @mentions from the editor before clearing
      const inlineMentions = inputRef.current?.getMentions() ?? [];

      onSubmit(
        trimmed,
        allComments.length > 0 ? allComments : undefined,
        messageAttachments.length > 0 ? messageAttachments : undefined,
        inlineMentions.length > 0 ? inlineMentions : undefined
      );
      // Clear editor imperatively (suppresses stale onUpdate)
      inputRef.current?.clear();
      setValue('');
      setAttachments([]);
      setHistoryIndex(-1);
      resetHeight();
      // Clear persisted draft after sending
      if (sessionId) {
        setChatDraftText(sessionId, '');
        setChatDraftContent(sessionId, null);
        setChatDraftAttachments(sessionId, []);
      }
    }, [onSubmit, isSending, sessionId, resetHeight]);

    // Merge images into context items
    const allItems = useMemo((): ContextItem[] => {
      const imageItems: ImageContextItem[] = attachments.map(att => ({
        kind: 'image' as const,
        id: `image:${att.id}`,
        label: `Image (${formatBytes(att.size)})`,
        attachment: att,
        onRemove: () => handleRemoveAttachment(att.id),
      }));
      return [...contextItems, ...imageItems];
    }, [contextItems, attachments, handleRemoveAttachment]);

    // Derived state
    const isDisabled = isStarting || isSending || isFailed;
    const hasPendingClarification = pendingClarification && onClarificationResolved;
    const hasPendingComments = pendingCommentsByFile && Object.keys(pendingCommentsByFile).length > 0;

    const inputPlaceholder =
      placeholder ||
      (isAgentBusy
        ? 'Queue more instructions...'
        : hasAgentCommands
          ? 'Ask to make changes, @mention files, run /commands'
          : 'Ask to make changes, @mention files');

    const showFocusHint = !isInputFocused && !hasPendingClarification && !hasPendingComments;

    // Queue slot
    const hasQueuedMessage = isQueued && queuedMessage && onCancelQueue && updateQueueContent;
    const hasTodos = todoItems.length > 0;

    // Determine if context zone should show
    const hasContextZone =
      hasQueuedMessage ||
      hasTodos ||
      allItems.length > 0 ||
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
            workflowId={null}
            defaultStepId={null}
            steps={[]}
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

        {/* CONTEXT ZONE: scrollable, above editor */}
        {hasContextZone && (
          <ContextZone
            items={allItems}
            sessionId={sessionId}
            queueSlot={
              hasQueuedMessage ? (
                <QueuedMessageIndicator
                  ref={queuedMessageRef}
                  content={queuedMessage}
                  onCancel={onCancelQueue}
                  onUpdate={updateQueueContent}
                  isVisible={true}
                  onEditComplete={onQueueEditComplete}
                />
              ) : undefined
            }
            todoSlot={hasTodos ? <TodoSummary todos={todoItems} /> : undefined}
            clarificationSlot={
              hasPendingClarification ? (
                <div className="overflow-auto">
                  <ClarificationInputOverlay
                    message={pendingClarification}
                    onResolved={onClarificationResolved}
                  />
                </div>
              ) : undefined
            }
          />
        )}

        {/* EDITOR ZONE: flex-1, fills remaining space */}
        <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
          <Tooltip open={showRequestChangesTooltip}>
            <TooltipTrigger asChild>
              <div className="flex-1 min-h-0">
                <TipTapInput
                  ref={inputRef}
                  value={value}
                  onChange={handleChange}
                  onSubmit={handleSubmit}
                  placeholder={inputPlaceholder}
                  disabled={isDisabled || !!hasPendingClarification}
                  planModeEnabled={planModeEnabled}
                  submitKey={submitKey}
                  onFocus={() => setIsInputFocused(true)}
                  onBlur={() => setIsInputFocused(false)}
                  sessionId={sessionId}
                  taskId={taskId}
                  onAddContextFile={onAddContextFile}
                  onToggleContextFile={onToggleContextFile}
                  planContextEnabled={planContextEnabled}
                  onAgentCommand={handleAgentCommand}
                  onImagePaste={handleImagePaste}
                />
              </div>
            </TooltipTrigger>
            <TooltipContent side="top" className="bg-orange-600 text-white border-orange-700">
              <p className="font-medium">Write your changes here</p>
            </TooltipContent>
          </Tooltip>

          {/* Integrated toolbar */}
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
            contextCount={allItems.length}
            contextPopoverOpen={contextPopoverOpen}
            onContextPopoverOpenChange={setContextPopoverOpen}
            planContextEnabled={planContextEnabled}
            contextFiles={contextFiles}
            onToggleFile={onToggleContextFile}
          />
        </div>
      </div>
    );
  });
