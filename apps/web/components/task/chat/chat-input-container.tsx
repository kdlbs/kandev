"use client";

import { forwardRef, useCallback } from "react";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { ClarificationRequestMetadata, Message } from "@/lib/types/http";
import type { DiffComment } from "@/lib/diff/types";
import type { TaskMentionData } from "@/hooks/use-inline-mention";
import type { MCPAttachmentHistory } from "@/lib/state/slices/session-runtime/types";
import type { EntityReference } from "@/lib/types/entity-reference";
import { useChatInputContainer } from "./use-chat-input-container";
import { SessionStoppedBanner } from "./session-stopped-banner";
import { useSessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import {
  ChatInputBody,
  type ChatInputContextAreaProps,
  type ChatInputEditorAreaProps,
} from "./chat-input-body";
import type { ContextItem } from "@/lib/types/context";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { usePromptResultDelivery } from "@/hooks/use-prompt-result-delivery";
import { PromptResultRecovery } from "@/components/prompt-result-recovery";
import { t } from "@/lib/i18n";

// Re-export ImageAttachment type for consumers
export type { ImageAttachment } from "./image-attachment-preview";

// Type for message attachments sent to backend
export type MessageAttachment = {
  type: "image" | "audio" | "resource";
  data?: string;
  attachment_id?: string;
  mime_type: string;
  name?: string;
  size_bytes?: number;
  delivery_mode?: "prompt" | "path";
};

export type ChatInputContainerHandle = {
  focusInput: () => void;
  getTextareaElement: () => HTMLElement | null;
  getValue: () => string;
  getSelectionStart: () => number;
  insertText: (text: string, from: number, to: number) => void;
  clear: () => void;
  getAttachments: () => MessageAttachment[];
};

export type ChatSubmitResult = void | boolean | Promise<void | boolean>;

export type ChatSubmitPayload = {
  message: string;
  reviewComments?: DiffComment[];
  attachments?: MessageAttachment[];
  inlineMentions?: ContextFile[];
  inlineTaskMentions?: TaskMentionData[];
  entityReferences?: EntityReference[];
};

type ChatInputContainerProps = {
  onSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  sessionId: string | null;
  taskId: string | null;
  workspaceId?: string | null;
  entityReferencesEnabled?: boolean;
  taskTitle?: string;
  taskDescription: string;
  planModeEnabled: boolean;
  planModeAvailable?: boolean;
  mcpServers?: string[];
  mcpAttachmentHistory?: MCPAttachmentHistory;
  onPlanModeChange: (enabled: boolean) => void;
  isAgentBusy: boolean;
  /** True when a send would be delivered into the running turn (mid-turn
   * steering) rather than queued. Defaults to false. */
  supportsSteering?: boolean;
  isStarting: boolean;
  /** True only while a containerized executor is bootstrapping (Docker
   * prepare, Sprites sandbox spin-up). Distinct from the brief STARTING
   * state every session — including local quick-chat — passes through;
   * see useSessionState. */
  isPreparingEnvironment?: boolean;
  isMoving?: boolean;
  isSending: boolean;
  onCancel: () => void;
  placeholder?: string;
  pendingClarification?: Message | null;
  onClarificationResolved?: () => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  pendingCommentsByFile?: Record<string, DiffComment[]>;
  hasContextComments?: boolean;
  submitKey?: "enter" | "cmd_enter";
  hasAgentCommands?: boolean;
  isFailed?: boolean;
  isCompleted?: boolean;
  sessionErrorMessage?: string;
  needsRecovery?: boolean;
  executorUnavailable?: boolean;
  executorUnavailableReason?: string;
  contextItems?: ContextItem[];
  planContextEnabled?: boolean;
  contextFiles?: ContextFile[];
  onToggleContextFile?: (file: ContextFile) => void;
  onAddContextFile?: (file: ContextFile) => void;
  onImplementPlan?: (fresh: boolean) => void;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  hideAgentControls?: boolean;
  /** Hide the plan mode toggle button (for ephemeral/quick chat sessions) */
  hidePlanMode?: boolean;
};

type ContainerState = ReturnType<typeof useChatInputContainer>;
type NormalizedChatInputProps = ChatInputContainerProps & {
  isFailed: boolean;
  isCompleted: boolean;
  hasAgentCommands: boolean;
  submitKey: "enter" | "cmd_enter";
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  contextItems: ContextItem[];
  showRequestChangesTooltip: boolean;
  entityReferencesEnabled: boolean;
};

function normalizeChatInputProps(p: ChatInputContainerProps): NormalizedChatInputProps {
  return {
    ...p,
    isFailed: p.isFailed ?? false,
    isCompleted: p.isCompleted ?? false,
    hasAgentCommands: p.hasAgentCommands ?? false,
    submitKey: p.submitKey ?? "cmd_enter",
    planContextEnabled: p.planContextEnabled ?? false,
    contextFiles: p.contextFiles ?? [],
    contextItems: p.contextItems ?? [],
    showRequestChangesTooltip: p.showRequestChangesTooltip ?? false,
    entityReferencesEnabled: p.entityReferencesEnabled ?? false,
  };
}

function buildContextAreaProps(
  s: ContainerState,
  p: ChatInputContainerProps,
): ChatInputContextAreaProps {
  return {
    hasContextZone: s.hasContextZone,
    allItems: s.allItems,
    sessionId: p.sessionId,
  };
}

type EnhancePromptExtras = {
  onEnhancePrompt?: () => void;
  isEnhancingPrompt?: boolean;
  isUtilityConfigured?: boolean;
};

export function shouldShowCancelAgent(
  isAgentBusy: boolean,
  pendingClarification: Message | null | undefined,
): boolean {
  if (!pendingClarification) return isAgentBusy;
  return !(pendingClarification.metadata as ClarificationRequestMetadata | undefined)
    ?.agent_disconnected;
}

function buildEditorAreaProps(
  s: ContainerState,
  p: NormalizedChatInputProps,
  extras: EnhancePromptExtras = {},
): ChatInputEditorAreaProps {
  return {
    inputRef: s.inputRef,
    value: s.value,
    handleChange: s.handleChange,
    handleSubmitWithReset: s.handleSubmitWithReset,
    inputPlaceholder: s.inputPlaceholder,
    isDisabled: s.isDisabled,
    submitDisabled: s.submitDisabled,
    submitDisabledReason: s.submitDisabledReason,
    hasPendingAttachmentUploads: s.hasPendingAttachmentUploads,
    planModeEnabled: p.planModeEnabled,
    planModeAvailable: p.planModeAvailable ?? true,
    mcpServers: p.mcpServers ?? [],
    mcpAttachmentHistory: p.mcpAttachmentHistory,
    submitKey: p.submitKey,
    setIsInputFocused: s.setIsInputFocused,
    sessionId: p.sessionId,
    taskId: p.taskId,
    workspaceId: p.workspaceId ?? null,
    entityReferencesEnabled: p.entityReferencesEnabled,
    onAddContextFile: p.onAddContextFile,
    onToggleContextFile: p.onToggleContextFile,
    planContextEnabled: p.planContextEnabled,
    addFiles: s.addFiles,
    fileInputRef: s.fileInputRef,
    showRequestChangesTooltip: p.showRequestChangesTooltip,
    isAgentBusy: p.isAgentBusy || !!(p.pendingClarification && p.onClarificationResolved),
    canCancelAgent: shouldShowCancelAgent(p.isAgentBusy, p.pendingClarification),
    onPlanModeChange: p.onPlanModeChange,
    taskTitle: p.taskTitle,
    taskDescription: p.taskDescription,
    isSending: p.isSending,
    onCancel: p.onCancel,
    contextCount: s.allItems.length,
    contextPopoverOpen: s.contextPopoverOpen,
    setContextPopoverOpen: s.setContextPopoverOpen,
    contextFiles: p.contextFiles,
    onImplementPlan: p.onImplementPlan,
    onEnhancePrompt: extras.onEnhancePrompt,
    isEnhancingPrompt: extras.isEnhancingPrompt,
    isUtilityConfigured: extras.isUtilityConfigured,
    hideSessionsDropdown: p.hideSessionsDropdown,
    minimalToolbar: p.minimalToolbar,
    hideAgentControls: p.hideAgentControls,
    hidePlanMode: p.hidePlanMode,
  };
}

type StoppedBannerSource = Pick<
  ChatInputContainerProps,
  "executorUnavailable" | "executorUnavailableReason" | "sessionErrorMessage"
>;

export function buildStoppedBannerProps(p: StoppedBannerSource) {
  if (!p.executorUnavailable) {
    return p.sessionErrorMessage ? { message: p.sessionErrorMessage } : {};
  }
  return {
    message: p.sessionErrorMessage ?? t("task:executorEnvironmentIsUnavailable"),
    detail: p.executorUnavailableReason,
    resumeLabel: t("task:restart"),
    resumingLabel: t("task:restarting"),
  };
}

function applyEnhancedPromptToEditor(inputRef: ContainerState["inputRef"], value: string): boolean {
  const input = inputRef.current;
  if (!input) {
    return false;
  }

  input.setValue(value);
  return input.getValue() === value;
}

function useChatPromptEnhancement({
  inputRef,
  taskId,
  sessionId,
  taskTitle,
  taskDescription,
}: {
  inputRef: ContainerState["inputRef"];
  taskId: string | null;
  sessionId: string | null;
  taskTitle?: string;
  taskDescription: string;
}) {
  const isUtilityConfigured = useIsUtilityConfigured();
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId,
    taskTitle,
    taskDescription,
  });
  const getCurrentEditorValue = useCallback(() => inputRef.current?.getValue() ?? null, [inputRef]);
  const applyEnhancedPrompt = useCallback(
    (nextValue: string) => applyEnhancedPromptToEditor(inputRef, nextValue),
    [inputRef],
  );
  const promptDelivery = usePromptResultDelivery({
    scopeKey: `chat:${taskId ?? ""}:${sessionId ?? ""}`,
    getCurrent: getCurrentEditorValue,
    apply: applyEnhancedPrompt,
  });
  const handleEnhancePrompt = useCallback(() => {
    const currentValue = getCurrentEditorValue();
    if (currentValue === null) return;
    if (!currentValue.trim()) return;
    const generation = promptDelivery.captureScope();
    void enhancePrompt(currentValue, (result) =>
      promptDelivery.deliver(currentValue, result, generation),
    );
  }, [getCurrentEditorValue, enhancePrompt, promptDelivery]);

  return { handleEnhancePrompt, isEnhancingPrompt, isUtilityConfigured, promptDelivery };
}

export const ChatInputContainer = forwardRef<ChatInputContainerHandle, ChatInputContainerProps>(
  // eslint-disable-next-line complexity -- top-level component chooses the stopped or editor surface after shared hook setup.
  function ChatInputContainer(props, ref) {
    const { sessionId, taskId, taskTitle, taskDescription, isAgentBusy, isStarting, isSending } =
      props;
    const p = normalizeChatInputProps(props);
    const isMoving = props.isMoving ?? false;
    const executorUnavailable = props.executorUnavailable ?? false;
    const isBusyVisual = isStarting || isMoving;

    const s = useChatInputContainer({
      ref,
      sessionId,
      workspaceId: props.workspaceId,
      isSending,
      isStarting,
      isPreparingEnvironment: props.isPreparingEnvironment ?? false,
      isMoving,
      isFailed: p.isFailed,
      needsRecovery: props.needsRecovery ?? false,
      executorUnavailable,
      isAgentBusy,
      supportsSteering: props.supportsSteering ?? false,
      hasAgentCommands: p.hasAgentCommands,
      placeholder: props.placeholder,
      contextItems: p.contextItems,
      pendingClarification: props.pendingClarification,
      onClarificationResolved: props.onClarificationResolved,
      pendingCommentsByFile: props.pendingCommentsByFile,
      hasContextComments: props.hasContextComments ?? false,
      showRequestChangesTooltip: p.showRequestChangesTooltip,
      onRequestChangesTooltipDismiss: props.onRequestChangesTooltipDismiss,
      onSubmit: props.onSubmit,
    });

    const recoveryActions = useSessionRecoveryActions({
      taskId: taskId ?? "",
      sessionId: sessionId ?? "",
    });

    const promptEnhancement = useChatPromptEnhancement({
      inputRef: s.inputRef,
      taskId,
      sessionId,
      taskTitle,
      taskDescription,
    });

    if (p.isFailed || p.isCompleted || executorUnavailable) {
      return (
        <SessionStoppedBanner
          mode={p.isCompleted ? "completed" : "recoverable"}
          showDialog={s.showNewSessionDialog}
          onShowDialog={s.setShowNewSessionDialog}
          taskId={taskId}
          sessionId={sessionId}
          workspaceId={props.workspaceId}
          recoveryActions={recoveryActions}
          {...buildStoppedBannerProps(props)}
        />
      );
    }

    return (
      <ChatInputBody
        containerRef={s.containerRef}
        height={s.height}
        resizeHandleProps={s.resizeHandleProps}
        isStarting={isBusyVisual}
        isAgentBusy={isAgentBusy}
        hasClarification={s.hasClarification}
        showRequestChangesTooltip={p.showRequestChangesTooltip}
        hasPendingComments={s.hasPendingComments}
        planModeEnabled={props.planModeEnabled}
        showFocusHint={s.showFocusHint}
        needsRecovery={(props.needsRecovery ?? false) || executorUnavailable}
        addFiles={s.addFiles}
        contextAreaProps={buildContextAreaProps(s, p)}
        promptResultRecovery={
          promptEnhancement.promptDelivery.pendingResult ? (
            <PromptResultRecovery
              pendingResult={promptEnhancement.promptDelivery.pendingResult}
              onApply={promptEnhancement.promptDelivery.applyPending}
              onCopy={promptEnhancement.promptDelivery.copyPending}
            />
          ) : null
        }
        editorAreaProps={buildEditorAreaProps(s, p, {
          onEnhancePrompt: promptEnhancement.handleEnhancePrompt,
          isEnhancingPrompt: promptEnhancement.isEnhancingPrompt,
          isUtilityConfigured: promptEnhancement.isUtilityConfigured,
        })}
      />
    );
  },
);
