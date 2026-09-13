"use client";

import { useCallback, useRef, useState, useEffect, useImperativeHandle } from "react";
import type React from "react";
import { useTranslation } from "react-i18next";
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
import { t } from "@/lib/i18n";
import {
  useComposerActivity,
  useComposerDisclosureContext,
  useComposerFocus,
} from "./composer-disclosure";
import type { ComposerActivity } from "./use-composer-disclosure";

type UseChatInputContainerParams = {
  ref: React.ForwardedRef<ChatInputContainerHandle>;
  sessionId: string | null;
  workspaceId?: string | null;
  isSending: boolean;
  isStarting: boolean;
  /** True when the selected startup session has a complete queue identity. */
  canQueueWhileStarting: boolean;
  /** True only during a real Docker/Sprites prepare phase. Different from
   * `isStarting`, which fires for every session that's transitioning
   * through STARTING (including local quick-chat). Drives the "agent still
   * being set up" submit-disabled tooltip so it only appears when a
   * container/sandbox is genuinely bootstrapping; startup submission also
   * requires a complete queue identity. */
  isPreparingEnvironment: boolean;
  isMoving: boolean;
  isFailed: boolean;
  needsRecovery: boolean;
  executorUnavailable: boolean;
  isAgentBusy: boolean;
  // supportsSteering is true when a send would be delivered into the running
  // turn (mid-turn steering) instead of queued. Drives the composer's
  // delivery-now affordance.
  supportsSteering: boolean;
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
  const focusInput = useComposerFocus(inputRef);
  useImperativeHandle(
    ref,
    () => ({
      focusInput,
      getTextareaElement: () => inputRef.current?.getTextareaElement() ?? null,
      getValue: () => inputRef.current?.getValue() ?? "",
      getSelectionStart: () => inputRef.current?.getSelectionStart() ?? 0,
      insertText: (text: string, from: number, to: number) => {
        inputRef.current?.insertText(text, from, to);
      },
      clear: () => inputRef.current?.clear(),
      getAttachments,
    }),
    [inputRef, getAttachments, focusInput],
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
  steerPlaceholder: string | undefined,
): string {
  if (isStarting) return t("task:preparingWorkspace");
  // The steer label wins over a caller-supplied placeholder: when a send would be
  // delivered into the running turn, that is the one thing the operator most needs
  // to know, and it must not be masked by a generic "Continue working…" prompt the
  // task composer passes. It promises delivery, never that the agent will fold it.
  // steerPlaceholder is set only when the session is steer-eligible, so this
  // branch is inert for every non-steering composer.
  if (steerPlaceholder) return steerPlaceholder;
  if (placeholder) return placeholder;
  if (isAgentBusy) return t("task:queueMoreInstructions");
  if (hasAgentCommands) return t("task:askToMakeChangesWithCommands");
  return t("task:askToMakeChanges");
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
  hasPendingAttachmentUploads: boolean;
  isInputFocused: boolean;
  value: string;
  placeholder: string | undefined;
  isAgentBusy: boolean;
  hasAgentCommands: boolean;
  steerPlaceholder: string | undefined;
  canQueueWhileStarting: boolean;
}) {
  const hasClarification = !!(params.pendingClarification && params.onClarificationResolved);
  // Keep the editor available during STARTING so the user can prepare a draft.
  // Queue-capable sessions may submit during that state. Preparation-only
  // status and sessions without an immutable queue identity remain blocked.
  const startupSubmitDisabled =
    params.isStarting && !hasClarification && !params.canQueueWhileStarting;
  const isDisabled =
    params.isMoving ||
    params.isSending ||
    params.isFailed ||
    params.needsRecovery ||
    params.executorUnavailable;
  const submitDisabled = isDisabled || startupSubmitDisabled || params.hasPendingAttachmentUploads;
  // The "agent still being set up" tooltip is only meaningful while a
  // container/sandbox is actively bootstrapping. The brief STARTING
  // transition for local quick-chat sessions doesn't deserve its own
  // tooltip. The disabled send action is sufficient feedback.
  const submitDisabledReason =
    (isDisabled || startupSubmitDisabled) && params.isPreparingEnvironment
      ? t("task:agentStillBeingSetUp")
      : undefined;
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
    startupSubmitDisabled,
    params.steerPlaceholder,
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

function useComposerInputPresentation({
  inputRef,
  addFiles,
  showRequestChangesTooltip,
  ...activity
}: ComposerActivity & {
  inputRef: React.RefObject<TipTapInputHandle | null>;
  addFiles: ReturnType<typeof useChatInputState>["addFiles"];
  showRequestChangesTooltip: boolean;
}) {
  const [processingFiles, setProcessingFiles] = useState(0);
  const visible = useComposerDisclosureContext()?.expanded !== false;
  useComposerActivity({ ...activity, busy: activity.busy || processingFiles > 0 });
  useEffect(() => {
    if (visible && showRequestChangesTooltip) inputRef.current?.focus();
  }, [visible, showRequestChangesTooltip, inputRef]);
  return useCallback(
    async (...args: Parameters<typeof addFiles>) => {
      setProcessingFiles((count) => count + 1);
      try {
        await addFiles(...args);
      } finally {
        setProcessingFiles((count) => count - 1);
      }
    },
    [addFiles],
  );
}

export function useChatInputContainer(params: UseChatInputContainerParams) {
  const { t } = useTranslation("chat");
  const { ref, sessionId, isSending, isStarting, isPreparingEnvironment, isMoving } = params;
  const { isFailed, needsRecovery, executorUnavailable, isAgentBusy, hasAgentCommands } = params;
  const { supportsSteering } = params;
  const { placeholder, pendingClarification, onClarificationResolved } = params;
  const { pendingCommentsByFile, showRequestChangesTooltip } = params;

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
  const {
    value,
    attachments,
    inputRef,
    addFiles,
    handleChange,
    handleSubmit,
    allItems,
    getAttachments,
    hasPendingAttachmentUploads,
  } = useChatInputState(params);

  useSyncTipTapRef(tiptapRef, inputRef);

  useInputHandle(ref, inputRef, getAttachments);
  const addFilesWithHold = useComposerInputPresentation({
    inputRef,
    addFiles,
    showRequestChangesTooltip,
    draft: value.trim().length > 0 || attachments.length > 0 || params.hasContextComments,
    busy: isSending || isMoving || hasPendingAttachmentUploads,
    required: isFailed || needsRecovery || executorUnavailable || Boolean(pendingClarification),
    overlay: contextPopoverOpen || showNewSessionDialog,
  });

  // Auto-expand the input container as the user types more lines
  const handleChangeWithAutoExpand = useCallback(
    (val: string) => {
      handleChange(val);
      requestAnimationFrame(autoExpand);
    },
    [handleChange, autoExpand],
  );

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
    hasPendingAttachmentUploads,
    isInputFocused,
    value,
    placeholder,
    isAgentBusy,
    hasAgentCommands,
    steerPlaceholder: supportsSteering ? t("chat:composerSteerPlaceholder") : undefined,
    canQueueWhileStarting: params.canQueueWhileStarting,
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
    addFiles: addFilesWithHold,
    fileInputRef,
    handleChange: handleChangeWithAutoExpand,
    handleSubmitWithReset,
    allItems,
    hasPendingAttachmentUploads,
    ...derived,
  };
}

export type { TipTapInputHandle };
