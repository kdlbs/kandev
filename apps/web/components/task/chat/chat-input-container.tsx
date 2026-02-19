"use client";

import { useCallback, useState, useEffect, forwardRef, useImperativeHandle } from "react";
import { IconAlertTriangle, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { TipTapInput } from "./tiptap-input";
import { ClarificationInputOverlay } from "./clarification-input-overlay";
import { ChatInputFocusHint } from "./chat-input-focus-hint";
import { ResizeHandle } from "./resize-handle";
import { TodoSummary } from "./todo-summary";
import {
  QueuedMessageIndicator,
  type QueuedMessageIndicatorHandle,
} from "./queued-message-indicator";
import { ChatInputToolbar } from "./chat-input-toolbar";
import { ContextZone } from "./context-items/context-zone";
import type { ContextItem } from "@/lib/types/context";
import type { ContextFile } from "@/lib/state/context-files-store";
import { useResizableInput } from "@/hooks/use-resizable-input";
import type { Message } from "@/lib/types/http";
import type { DiffComment } from "@/lib/diff/types";
import { useChatInputState } from "./use-chat-input-state";

// Re-export ImageAttachment type for consumers
export type { ImageAttachment } from "./image-attachment-preview";

// Type for message attachments sent to backend
export type MessageAttachment = {
  type: "image";
  data: string; // Base64 data
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
  onSubmit: (
    message: string,
    reviewComments?: DiffComment[],
    attachments?: MessageAttachment[],
    inlineMentions?: ContextFile[],
  ) => void;
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
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  submitKey?: "enter" | "cmd_enter";
  hasAgentCommands?: boolean;
  isFailed?: boolean;
  isQueued?: boolean;
  contextItems?: ContextItem[];
  planContextEnabled?: boolean;
  contextFiles?: ContextFile[];
  onToggleContextFile?: (file: ContextFile) => void;
  onAddContextFile?: (file: ContextFile) => void;
  todoItems?: TodoItem[];
  queuedMessage?: string | null;
  onCancelQueue?: () => void;
  updateQueueContent?: (content: string) => Promise<void>;
  queuedMessageRef?: React.RefObject<QueuedMessageIndicatorHandle | null>;
  onQueueEditComplete?: () => void;
  isPanelFocused?: boolean;
};

function FailedSessionBanner({
  showDialog,
  onShowDialog,
  taskId,
  taskTitle,
  taskDescription,
}: {
  showDialog: boolean;
  onShowDialog: (open: boolean) => void;
  taskId: string | null;
  taskTitle?: string;
  taskDescription: string;
}) {
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
            onClick={() => onShowDialog(true)}
          >
            <IconPlus className="h-3.5 w-3.5" />
            New Session
          </Button>
        </div>
      </div>
      <TaskCreateDialog
        open={showDialog}
        onOpenChange={onShowDialog}
        mode="session"
        workspaceId={null}
        workflowId={null}
        defaultStepId={null}
        steps={[]}
        taskId={taskId}
        initialValues={{ title: taskTitle ?? "", description: taskDescription }}
      />
    </>
  );
}

function getInputPlaceholder(
  placeholder: string | undefined,
  isAgentBusy: boolean,
  hasAgentCommands: boolean,
): string {
  if (placeholder) return placeholder;
  if (isAgentBusy) return "Queue more instructions...";
  if (hasAgentCommands) return "Ask to make changes, @mention files, run /commands";
  return "Ask to make changes, @mention files";
}

function hasPendingCommentFiles(
  pendingCommentsByFile: Record<string, DiffComment[]> | undefined,
): boolean {
  return !!(pendingCommentsByFile && Object.keys(pendingCommentsByFile).length > 0);
}

type DerivedInputState = {
  isDisabled: boolean;
  hasClarification: boolean;
  hasPendingComments: boolean;
  hasQueuedMessage: boolean;
  hasTodos: boolean;
  hasContextZone: boolean;
  showFocusHint: boolean;
  inputPlaceholder: string;
};

type DerivedInputParams = {
  isStarting: boolean;
  isSending: boolean;
  isFailed: boolean;
  pendingClarification: Message | null | undefined;
  onClarificationResolved: (() => void) | undefined;
  pendingCommentsByFile: Record<string, DiffComment[]> | undefined;
  isQueued: boolean;
  queuedMessage: string | null | undefined;
  onCancelQueue: (() => void) | undefined;
  updateQueueContent: ((content: string) => Promise<void>) | undefined;
  todoItems: { text: string }[];
  allItemsLength: number;
  isInputFocused: boolean;
  placeholder: string | undefined;
  isAgentBusy: boolean;
  hasAgentCommands: boolean;
};

function computeDerivedInputState(p: DerivedInputParams): DerivedInputState {
  const isDisabled = p.isStarting || p.isSending || p.isFailed;
  const hasClarification = !!(p.pendingClarification && p.onClarificationResolved);
  const hasPendingComments = hasPendingCommentFiles(p.pendingCommentsByFile);
  const hasQueuedMessage = !!(
    p.isQueued &&
    p.queuedMessage &&
    p.onCancelQueue &&
    p.updateQueueContent
  );
  const hasTodos = p.todoItems.length > 0;
  const hasContextZone = hasQueuedMessage || hasTodos || p.allItemsLength > 0 || hasClarification;
  const showFocusHint = !p.isInputFocused && !hasClarification && !hasPendingComments;
  const inputPlaceholder = getInputPlaceholder(p.placeholder, p.isAgentBusy, p.hasAgentCommands);
  return {
    isDisabled,
    hasClarification,
    hasPendingComments,
    hasQueuedMessage,
    hasTodos,
    hasContextZone,
    showFocusHint,
    inputPlaceholder,
  };
}

type ChatInputBodyProps = {
  containerRef: React.RefObject<HTMLDivElement | null>;
  height: React.CSSProperties["height"];
  resizeHandleProps: { onMouseDown: (e: React.MouseEvent) => void; onDoubleClick: () => void };
  isPanelFocused: boolean | undefined;
  isInputFocused: boolean;
  isAgentBusy: boolean;
  hasClarification: boolean;
  showRequestChangesTooltip: boolean;
  hasPendingComments: boolean;
  showFocusHint: boolean;
  hasContextZone: boolean;
  allItems: ContextItem[];
  sessionId: string | null;
  hasQueuedMessage: boolean;
  queuedMessage?: string | null;
  onCancelQueue?: () => void;
  updateQueueContent?: (content: string) => Promise<void>;
  queuedMessageRef?: React.RefObject<QueuedMessageIndicatorHandle | null>;
  onQueueEditComplete?: () => void;
  hasTodos: boolean;
  todoItems: { text: string; done?: boolean }[];
  pendingClarification?: Message | null;
  onClarificationResolved?: () => void;
  value: string;
  inputRef: React.RefObject<import("./tiptap-input").TipTapInputHandle | null>;
  handleChange: (val: string) => void;
  handleSubmitWithReset: () => void;
  inputPlaceholder: string;
  isDisabled: boolean;
  planModeEnabled: boolean;
  submitKey: "enter" | "cmd_enter";
  setIsInputFocused: (focused: boolean) => void;
  taskId: string | null;
  onAddContextFile?: (file: ContextFile) => void;
  onToggleContextFile?: (file: ContextFile) => void;
  planContextEnabled: boolean;
  handleAgentCommand: (command: string) => void;
  handleImagePaste: (files: File[]) => Promise<void>;
  onPlanModeChange: (enabled: boolean) => void;
  taskTitle?: string;
  taskDescription: string;
  isSending: boolean;
  onCancel: () => void;
  contextCount: number;
  contextPopoverOpen: boolean;
  setContextPopoverOpen: (open: boolean) => void;
  contextFiles: ContextFile[];
};

function ChatInputBody({
  containerRef,
  height,
  resizeHandleProps,
  isPanelFocused,
  isInputFocused,
  isAgentBusy,
  hasClarification,
  showRequestChangesTooltip,
  hasPendingComments,
  showFocusHint,
  hasContextZone,
  allItems,
  sessionId,
  hasQueuedMessage,
  queuedMessage,
  onCancelQueue,
  updateQueueContent,
  queuedMessageRef,
  onQueueEditComplete,
  hasTodos,
  todoItems,
  pendingClarification,
  onClarificationResolved,
  value,
  inputRef,
  handleChange,
  handleSubmitWithReset,
  inputPlaceholder,
  isDisabled,
  planModeEnabled,
  submitKey,
  setIsInputFocused,
  taskId,
  onAddContextFile,
  onToggleContextFile,
  planContextEnabled,
  handleAgentCommand,
  handleImagePaste,
  onPlanModeChange,
  taskTitle,
  taskDescription,
  isSending,
  onCancel,
  contextCount,
  contextPopoverOpen,
  setContextPopoverOpen,
  contextFiles,
}: ChatInputBodyProps) {
  return (
    <div
      ref={containerRef}
      className={cn(
        "relative flex flex-col border rounded ",
        isPanelFocused ? "bg-background border-border" : "bg-background/40 border-border",
        isAgentBusy && "chat-input-running",
        hasClarification && "border-blue-500/50",
        showRequestChangesTooltip && "animate-pulse border-orange-500",
        hasPendingComments && "border-amber-500/50",
      )}
      style={{ height }}
    >
      <ResizeHandle visible={isPanelFocused || isInputFocused} {...resizeHandleProps} />
      <ChatInputFocusHint visible={showFocusHint} />
      {hasContextZone && (
        <ContextZone
          items={allItems}
          sessionId={sessionId}
          queueSlot={
            hasQueuedMessage ? (
              <QueuedMessageIndicator
                ref={queuedMessageRef}
                content={queuedMessage!}
                onCancel={onCancelQueue!}
                onUpdate={updateQueueContent!}
                isVisible={true}
                onEditComplete={onQueueEditComplete}
              />
            ) : undefined
          }
          todoSlot={hasTodos ? <TodoSummary todos={todoItems} /> : undefined}
          clarificationSlot={
            hasClarification ? (
              <div className="overflow-auto">
                <ClarificationInputOverlay
                  message={pendingClarification!}
                  onResolved={onClarificationResolved!}
                />
              </div>
            ) : undefined
          }
        />
      )}
      <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
        <Tooltip open={showRequestChangesTooltip}>
          <TooltipTrigger asChild>
            <div className="flex-1 min-h-0">
              <TipTapInput
                ref={inputRef}
                value={value}
                onChange={handleChange}
                onSubmit={handleSubmitWithReset}
                placeholder={inputPlaceholder}
                disabled={isDisabled || hasClarification}
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
          onSubmit={handleSubmitWithReset}
          submitKey={submitKey}
          contextCount={contextCount}
          contextPopoverOpen={contextPopoverOpen}
          onContextPopoverOpenChange={setContextPopoverOpen}
          planContextEnabled={planContextEnabled}
          contextFiles={contextFiles}
          onToggleFile={onToggleContextFile}
        />
      </div>
    </div>
  );
}

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
      submitKey = "cmd_enter",
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
    ref,
  ) {
    const [isInputFocused, setIsInputFocused] = useState(false);
    const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
    const [contextPopoverOpen, setContextPopoverOpen] = useState(false);
    const { height, resetHeight, containerRef, resizeHandleProps } = useResizableInput(
      sessionId ?? undefined,
    );
    const { value, inputRef, handleImagePaste, handleChange, handleSubmit, allItems } =
      useChatInputState({
        sessionId,
        isSending,
        contextItems,
        pendingCommentsByFile,
        showRequestChangesTooltip,
        onRequestChangesTooltipDismiss,
        onSubmit,
      });

    useImperativeHandle(
      ref,
      () => ({
        focusInput: () => inputRef.current?.focus(),
        getTextareaElement: () => inputRef.current?.getTextareaElement() ?? null,
        getValue: () => inputRef.current?.getValue() ?? "",
        getSelectionStart: () => inputRef.current?.getSelectionStart() ?? 0,
        insertText: (text: string, from: number, to: number) => {
          inputRef.current?.insertText(text, from, to);
        },
      }),
      [inputRef],
    );

    useEffect(() => {
      if (showRequestChangesTooltip && inputRef.current) inputRef.current.focus();
    }, [showRequestChangesTooltip, inputRef]);

    const handleAgentCommand = useCallback(
      (commandName: string) => {
        onSubmit(`/${commandName}`);
      },
      [onSubmit],
    );
    const handleSubmitWithReset = useCallback(
      () => handleSubmit(resetHeight),
      [handleSubmit, resetHeight],
    );

    const {
      isDisabled,
      hasClarification,
      hasPendingComments,
      hasQueuedMessage,
      hasTodos,
      hasContextZone,
      showFocusHint,
      inputPlaceholder,
    } = computeDerivedInputState({
      isStarting,
      isSending,
      isFailed,
      pendingClarification,
      onClarificationResolved,
      pendingCommentsByFile,
      isQueued,
      queuedMessage,
      onCancelQueue,
      updateQueueContent,
      todoItems,
      allItemsLength: allItems.length,
      isInputFocused,
      placeholder,
      isAgentBusy,
      hasAgentCommands,
    });

    if (isFailed) {
      return (
        <FailedSessionBanner
          showDialog={showNewSessionDialog}
          onShowDialog={setShowNewSessionDialog}
          taskId={taskId}
          taskTitle={taskTitle}
          taskDescription={taskDescription}
        />
      );
    }

    return (
      <ChatInputBody
        containerRef={containerRef}
        height={height}
        resizeHandleProps={resizeHandleProps}
        isPanelFocused={isPanelFocused}
        isInputFocused={isInputFocused}
        isAgentBusy={isAgentBusy}
        hasClarification={hasClarification}
        showRequestChangesTooltip={showRequestChangesTooltip}
        hasPendingComments={hasPendingComments}
        showFocusHint={showFocusHint}
        hasContextZone={hasContextZone}
        allItems={allItems}
        sessionId={sessionId}
        hasQueuedMessage={hasQueuedMessage}
        queuedMessage={queuedMessage}
        onCancelQueue={onCancelQueue}
        updateQueueContent={updateQueueContent}
        queuedMessageRef={queuedMessageRef}
        onQueueEditComplete={onQueueEditComplete}
        hasTodos={hasTodos}
        todoItems={todoItems}
        pendingClarification={pendingClarification}
        onClarificationResolved={onClarificationResolved}
        value={value}
        inputRef={inputRef}
        handleChange={handleChange}
        handleSubmitWithReset={handleSubmitWithReset}
        inputPlaceholder={inputPlaceholder}
        isDisabled={isDisabled}
        planModeEnabled={planModeEnabled}
        submitKey={submitKey}
        setIsInputFocused={setIsInputFocused}
        taskId={taskId}
        onAddContextFile={onAddContextFile}
        onToggleContextFile={onToggleContextFile}
        planContextEnabled={planContextEnabled}
        handleAgentCommand={handleAgentCommand}
        handleImagePaste={handleImagePaste}
        onPlanModeChange={onPlanModeChange}
        taskTitle={taskTitle}
        taskDescription={taskDescription}
        isSending={isSending}
        onCancel={onCancel}
        contextCount={allItems.length}
        contextPopoverOpen={contextPopoverOpen}
        setContextPopoverOpen={setContextPopoverOpen}
        contextFiles={contextFiles}
      />
    );
  },
);
