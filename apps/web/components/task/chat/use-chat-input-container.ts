"use client";

import { useCallback, useRef, useState, useEffect, useImperativeHandle } from "react";
import type React from "react";
import { useResizableInput } from "@/hooks/use-resizable-input";
import { useChatInputState } from "./use-chat-input-state";
import type { TipTapInputHandle } from "./tiptap-input";
import type { ContextItem } from "@/lib/types/context";
import type { Message } from "@/lib/types/http";
import type { DiffComment } from "@/lib/diff/types";
import type {
  ChatSubmitPayload,
  ChatSubmitResult,
  MessageAttachment,
  ChatInputContainerHandle,
} from "./chat-input-container";

type UseChatInputContainerParams = {
  ref: React.ForwardedRef<ChatInputContainerHandle>;
  sessionId: string | null;
  isSending: boolean;
  isStarting: boolean;
  /** True only during a real Docker/Sprites prepare phase. Different from
   * `isStarting`, which fires for every session that's transitioning
   * through STARTING (including local quick-chat). Drives the "agent still
   * being set up" submit-disabled tooltip so it only appears when a
   * container/sandbox is genuinely bootstrapping; the disabled state
   * itself is still gated on the broader `isStarting` to keep e2e
   * Cmd+Enter from racing the not-yet-ready agent. */
  isPreparingEnvironment: boolean;
  isMoving: boolean;
  isFailed: boolean;
  needsRecovery: boolean;
  executorUnavailable: boolean;
  isAgentBusy: boolean;
  hasAgentCommands: boolean;
  placeholder: string | undefined;
  contextItems: ContextItem[];
  pendingClarification: Message | null | undefined;
  onClarificationResolved: (() => void) | undefined;
  pendingCommentsByFile: Record<string, DiffComment[]> | undefined;
  hasContextComments: boolean;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss: (() => void) | undefined;
  onSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
};

function useInputHandle(
  ref: React.ForwardedRef<ChatInputContainerHandle>,
  inputRef: React.RefObject<TipTapInputHandle | null>,
  getAttachments: () => MessageAttachment[],
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
      clear: () => inputRef.current?.clear(),
      getAttachments,
    }),
    [inputRef, getAttachments],
  );
}

function useSyncTipTapRef(
  tiptapRef: React.RefObject<TipTapInputHandle | null>,
  inputRef: React.RefObject<TipTapInputHandle | null>,
) {
  useEffect(() => {
    tiptapRef.current = inputRef.current;
  });
}

function getInputPlaceholder(
  placeholder: string | undefined,
  isAgentBusy: boolean,
  hasAgentCommands: boolean,
  isStarting: boolean,
): string {
  if (isStarting) return "Preparing workspace...";
  if (placeholder) return placeholder;
  if (isAgentBusy) return "Queue more instructions...";
  if (hasAgentCommands) return "Ask to make changes, @mention files, run /commands";
  return "Ask to make changes, @mention files";
}

export function shouldShowChatFocusHint(args: {
  isInputFocused: boolean;
  value: string;
  hasClarification: boolean;
  hasPendingComments: boolean;
}) {
  return (
    !args.isInputFocused &&
    args.value.trim().length === 0 &&
    !args.hasClarification &&
    !args.hasPendingComments
  );
}

function computeDerivedState(params: {
  isStarting: boolean;
  isPreparingEnvironment: boolean;
  isMoving: boolean;
  isSending: boolean;
  isFailed: boolean;
  needsRecovery: boolean;
  executorUnavailable: boolean;
  pendingClarification: Message | null | undefined;
  onClarificationResolved: (() => void) | undefined;
  pendingCommentsByFile: Record<string, DiffComment[]> | undefined;
  allItemsLength: number;
  isInputFocused: boolean;
  value: string;
  placeholder: string | undefined;
  isAgentBusy: boolean;
  hasAgentCommands: boolean;
}) {
  const hasClarification = !!(params.pendingClarification && params.onClarificationResolved);
  // STARTING blocks regular messages until the session reaches RUNNING. An
  // interactive clarification is different: its queue path is persistence-only,
  // so it remains safe while stale lifecycle metadata says STARTING.
  const isDisabled =
    (params.isStarting && !hasClarification) ||
    params.isMoving ||
    params.isSending ||
    params.isFailed ||
    params.needsRecovery ||
    params.executorUnavailable;
  const submitDisabled = isDisabled;
  // The "agent still being set up" tooltip is only meaningful while a
  // container/sandbox is actively bootstrapping. The brief STARTING
  // transition for local quick-chat sessions doesn't deserve its own
  // tooltip — the editor is disabled, that's the signal.
  const submitDisabledReason =
    isDisabled && params.isPreparingEnvironment ? "The agent is still being set up." : undefined;
  const hasPendingComments = !!(
    params.pendingCommentsByFile && Object.keys(params.pendingCommentsByFile).length > 0
  );
  const hasContextZone = params.allItemsLength > 0;
  const showFocusHint = shouldShowChatFocusHint({
    isInputFocused: params.isInputFocused,
    value: params.value,
    hasClarification,
    hasPendingComments,
  });
  const inputPlaceholder = getInputPlaceholder(
    params.placeholder,
    params.isAgentBusy,
    params.hasAgentCommands,
    params.isStarting && !hasClarification,
  );
  return {
    isDisabled,
    submitDisabled,
    submitDisabledReason,
    hasClarification,
    hasPendingComments,
    hasContextZone,
    showFocusHint,
    inputPlaceholder,
  };
}

export function useChatInputContainer(params: UseChatInputContainerParams) {
  const { ref, sessionId, isSending, isStarting, isPreparingEnvironment, isMoving } = params;
  const { isFailed, needsRecovery, executorUnavailable, isAgentBusy, hasAgentCommands } = params;
  const { placeholder, contextItems, pendingClarification, onClarificationResolved } = params;
  const { pendingCommentsByFile, showRequestChangesTooltip } = params;
  const { onRequestChangesTooltipDismiss, onSubmit } = params;

  const [isInputFocused, setIsInputFocused] = useState(false);
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
  const [contextPopoverOpen, setContextPopoverOpen] = useState(false);

  const tiptapRef = useRef<TipTapInputHandle | null>(null);
  const getContentElement = useCallback(() => tiptapRef.current?.getTextareaElement() ?? null, []);
  const { height, resetHeight, autoExpand, containerRef, resizeHandleProps } = useResizableInput(
    sessionId ?? undefined,
    getContentElement,
  );
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { value, inputRef, addFiles, handleChange, handleSubmit, allItems, getAttachments } =
    useChatInputState({
      sessionId,
      isSending,
      contextItems,
      pendingCommentsByFile,
      hasContextComments: params.hasContextComments,
      showRequestChangesTooltip,
      onRequestChangesTooltipDismiss,
      onSubmit,
    });

  useSyncTipTapRef(tiptapRef, inputRef);

  useInputHandle(ref, inputRef, getAttachments);

  // Auto-expand the input container as the user types more lines
  const handleChangeWithAutoExpand = useCallback(
    (val: string) => {
      handleChange(val);
      requestAnimationFrame(autoExpand);
    },
    [handleChange, autoExpand],
  );

  useEffect(() => {
    if (showRequestChangesTooltip && inputRef.current) inputRef.current.focus();
  }, [showRequestChangesTooltip, inputRef]);

  const handleSubmitWithReset = useCallback(
    () => handleSubmit(resetHeight),
    [handleSubmit, resetHeight],
  );

  const derived = computeDerivedState({
    isStarting,
    isPreparingEnvironment,
    isMoving,
    isSending,
    isFailed,
    needsRecovery,
    executorUnavailable,
    pendingClarification,
    onClarificationResolved,
    pendingCommentsByFile,
    allItemsLength: allItems.length,
    isInputFocused,
    value,
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
    addFiles,
    fileInputRef,
    handleChange: handleChangeWithAutoExpand,
    handleSubmitWithReset,
    allItems,
    ...derived,
  };
}

export type { TipTapInputHandle };
