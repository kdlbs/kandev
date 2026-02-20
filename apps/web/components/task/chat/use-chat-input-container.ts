"use client";

import { useCallback, useState, useEffect, useImperativeHandle } from "react";
import type React from "react";
import { useResizableInput } from "@/hooks/use-resizable-input";
import { useChatInputState } from "./use-chat-input-state";
import type { QueuedMessageIndicatorHandle } from "./queued-message-indicator";
import type { TipTapInputHandle } from "./tiptap-input";
import type { ContextItem } from "@/lib/types/context";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { Message } from "@/lib/types/http";
import type { DiffComment } from "@/lib/diff/types";
import type { MessageAttachment, ChatInputContainerHandle } from "./chat-input-container";

type UseChatInputContainerParams = {
  ref: React.ForwardedRef<ChatInputContainerHandle>;
  sessionId: string | null;
  isSending: boolean;
  isStarting: boolean;
  isFailed: boolean;
  isAgentBusy: boolean;
  hasAgentCommands: boolean;
  isQueued: boolean;
  placeholder: string | undefined;
  contextItems: ContextItem[];
  pendingClarification: Message | null | undefined;
  onClarificationResolved: (() => void) | undefined;
  pendingCommentsByFile: Record<string, DiffComment[]> | undefined;
  queuedMessage: string | null | undefined;
  onCancelQueue: (() => void) | undefined;
  updateQueueContent: ((content: string) => Promise<void>) | undefined;
  todoItems: { text: string; done?: boolean }[];
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss: (() => void) | undefined;
  onSubmit: (
    message: string,
    reviewComments?: DiffComment[],
    attachments?: MessageAttachment[],
    inlineMentions?: ContextFile[],
  ) => void;
  queuedMessageRef?: React.RefObject<QueuedMessageIndicatorHandle | null>;
};

function useInputHandle(
  ref: React.ForwardedRef<ChatInputContainerHandle>,
  inputRef: React.RefObject<TipTapInputHandle | null>,
) {
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

function computeDerivedState(params: {
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
}) {
  const isDisabled = params.isStarting || params.isSending || params.isFailed;
  const hasClarification = !!(params.pendingClarification && params.onClarificationResolved);
  const hasPendingComments = !!(
    params.pendingCommentsByFile && Object.keys(params.pendingCommentsByFile).length > 0
  );
  const hasQueuedMessage = !!(
    params.isQueued &&
    params.queuedMessage &&
    params.onCancelQueue &&
    params.updateQueueContent
  );
  const hasTodos = params.todoItems.length > 0;
  const hasContextZone =
    hasQueuedMessage || hasTodos || params.allItemsLength > 0 || hasClarification;
  const showFocusHint = !params.isInputFocused && !hasClarification && !hasPendingComments;
  const inputPlaceholder = getInputPlaceholder(
    params.placeholder,
    params.isAgentBusy,
    params.hasAgentCommands,
  );
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

export function useChatInputContainer(params: UseChatInputContainerParams) {
  const {
    ref,
    sessionId,
    isSending,
    isStarting,
    isFailed,
    isAgentBusy,
    hasAgentCommands,
    isQueued,
    placeholder,
    contextItems,
    pendingClarification,
    onClarificationResolved,
    pendingCommentsByFile,
    queuedMessage,
    onCancelQueue,
    updateQueueContent,
    todoItems,
    showRequestChangesTooltip,
    onRequestChangesTooltipDismiss,
    onSubmit,
  } = params;

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

  useInputHandle(ref, inputRef);

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

  const derived = computeDerivedState({
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

  return {
    isInputFocused,
    setIsInputFocused,
    showNewSessionDialog,
    setShowNewSessionDialog,
    contextPopoverOpen,
    setContextPopoverOpen,
    height,
    containerRef,
    resizeHandleProps,
    value,
    inputRef,
    handleImagePaste,
    handleChange,
    handleSubmitWithReset,
    handleAgentCommand,
    allItems,
    ...derived,
  };
}

export type { TipTapInputHandle };
