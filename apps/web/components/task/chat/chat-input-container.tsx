"use client";

import { forwardRef } from "react";
import { IconAlertTriangle, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { Message } from "@/lib/types/http";
import type { DiffComment } from "@/lib/diff/types";
import type { QueuedMessageIndicatorHandle } from "./queued-message-indicator";
import { useChatInputContainer } from "./use-chat-input-container";
import { ChatInputBody, type ChatInputContextAreaProps, type ChatInputEditorAreaProps } from "./chat-input-body";
import type { ContextItem } from "@/lib/types/context";

// Re-export ImageAttachment type for consumers
export type { ImageAttachment } from "./image-attachment-preview";

// Type for message attachments sent to backend
export type MessageAttachment = {
  type: "image";
  data: string;
  mime_type: string;
};

export type ChatInputContainerHandle = {
  focusInput: () => void;
  getTextareaElement: () => HTMLElement | null;
  getValue: () => string;
  getSelectionStart: () => number;
  insertText: (text: string, from: number, to: number) => void;
};

type TodoItem = { text: string; done?: boolean };

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

type ContainerState = ReturnType<typeof useChatInputContainer>;

function buildContextAreaProps(
  s: ContainerState,
  p: ChatInputContainerProps,
): ChatInputContextAreaProps {
  return {
    hasContextZone: s.hasContextZone,
    allItems: s.allItems,
    sessionId: p.sessionId,
    hasQueuedMessage: s.hasQueuedMessage,
    queuedMessage: p.queuedMessage,
    onCancelQueue: p.onCancelQueue,
    updateQueueContent: p.updateQueueContent,
    queuedMessageRef: p.queuedMessageRef,
    onQueueEditComplete: p.onQueueEditComplete,
    hasTodos: s.hasTodos,
    todoItems: p.todoItems ?? [],
    hasClarification: s.hasClarification,
    pendingClarification: p.pendingClarification,
    onClarificationResolved: p.onClarificationResolved,
  };
}

function buildEditorAreaProps(
  s: ContainerState,
  p: ChatInputContainerProps,
): ChatInputEditorAreaProps {
  return {
    inputRef: s.inputRef,
    value: s.value,
    handleChange: s.handleChange,
    handleSubmitWithReset: s.handleSubmitWithReset,
    inputPlaceholder: s.inputPlaceholder,
    isDisabled: s.isDisabled,
    hasClarification: s.hasClarification,
    planModeEnabled: p.planModeEnabled,
    submitKey: p.submitKey ?? "cmd_enter",
    setIsInputFocused: s.setIsInputFocused,
    sessionId: p.sessionId,
    taskId: p.taskId,
    onAddContextFile: p.onAddContextFile,
    onToggleContextFile: p.onToggleContextFile,
    planContextEnabled: p.planContextEnabled ?? false,
    handleAgentCommand: s.handleAgentCommand,
    handleImagePaste: s.handleImagePaste,
    showRequestChangesTooltip: p.showRequestChangesTooltip ?? false,
    isAgentBusy: p.isAgentBusy,
    onPlanModeChange: p.onPlanModeChange,
    taskTitle: p.taskTitle,
    taskDescription: p.taskDescription,
    isSending: p.isSending,
    onCancel: p.onCancel,
    contextCount: s.allItems.length,
    contextPopoverOpen: s.contextPopoverOpen,
    setContextPopoverOpen: s.setContextPopoverOpen,
    contextFiles: p.contextFiles ?? [],
  };
}

export const ChatInputContainer = forwardRef<ChatInputContainerHandle, ChatInputContainerProps>(
  function ChatInputContainer(props, ref) {
    const {
      sessionId, taskId, taskTitle, taskDescription, isAgentBusy,
      isStarting, isSending, isFailed = false, isPanelFocused,
      showRequestChangesTooltip = false,
    } = props;

    const p = {
      ...props,
      isFailed: isFailed ?? false,
      isQueued: props.isQueued ?? false,
      hasAgentCommands: props.hasAgentCommands ?? false,
      submitKey: props.submitKey ?? "cmd_enter",
      planContextEnabled: props.planContextEnabled ?? false,
      contextFiles: props.contextFiles ?? [],
      contextItems: props.contextItems ?? [],
      todoItems: props.todoItems ?? [],
      showRequestChangesTooltip,
    } as const;

    const s = useChatInputContainer({
      ref, sessionId, isSending, isStarting, isFailed: p.isFailed, isAgentBusy,
      hasAgentCommands: p.hasAgentCommands, isQueued: p.isQueued,
      placeholder: props.placeholder, contextItems: p.contextItems,
      pendingClarification: props.pendingClarification,
      onClarificationResolved: props.onClarificationResolved,
      pendingCommentsByFile: props.pendingCommentsByFile,
      queuedMessage: props.queuedMessage, onCancelQueue: props.onCancelQueue,
      updateQueueContent: props.updateQueueContent, todoItems: p.todoItems,
      showRequestChangesTooltip, onRequestChangesTooltipDismiss: props.onRequestChangesTooltipDismiss,
      onSubmit: props.onSubmit, queuedMessageRef: props.queuedMessageRef,
    });

    if (p.isFailed) {
      return (
        <FailedSessionBanner
          showDialog={s.showNewSessionDialog}
          onShowDialog={s.setShowNewSessionDialog}
          taskId={taskId}
          taskTitle={taskTitle}
          taskDescription={taskDescription}
        />
      );
    }

    return (
      <ChatInputBody
        containerRef={s.containerRef}
        height={s.height}
        resizeHandleProps={s.resizeHandleProps}
        isPanelFocused={isPanelFocused}
        isInputFocused={s.isInputFocused}
        isAgentBusy={isAgentBusy}
        hasClarification={s.hasClarification}
        showRequestChangesTooltip={showRequestChangesTooltip}
        hasPendingComments={s.hasPendingComments}
        showFocusHint={s.showFocusHint}
        contextAreaProps={buildContextAreaProps(s, p)}
        editorAreaProps={buildEditorAreaProps(s, p)}
      />
    );
  },
);
