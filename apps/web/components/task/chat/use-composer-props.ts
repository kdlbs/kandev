"use client";

import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import type { ChatPanelState } from "./use-chat-panel-state";

type ComposerPropsArgs = {
  panelState: ChatPanelState;
  composerWorkspaceId: string | null;
  isMoving: boolean;
  implementPlanHandler: ((fresh: boolean) => Promise<void> | Promise<boolean>) | undefined;
  executor: { unavailable: boolean; reason?: string };
  placeholder: string;
  handleSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  handleCancelTurn: () => Promise<void>;
  isSending: boolean;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  onClarificationResolved: () => void;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  hideAgentControls?: boolean;
  hidePlanMode?: boolean;
};

/**
 * Builds the (large) prop object for `ChatInputContainer` from panel state
 * and derived values. Kept separate so `ChatInputArea`'s render body stays
 * short — behavior is identical to inlining every prop.
 */
export function useComposerProps(args: ComposerPropsArgs) {
  const {
    panelState,
    composerWorkspaceId,
    isMoving,
    implementPlanHandler,
    executor,
    placeholder,
    handleSubmit,
    handleCancelTurn,
    isSending,
    showRequestChangesTooltip,
    onRequestChangesTooltipDismiss,
    onClarificationResolved,
    hideSessionsDropdown,
    minimalToolbar,
    hideAgentControls,
    hidePlanMode,
  } = args;
  const { resolvedSessionId, taskId, isAgentBusy, needsRecovery, planModeEnabled } = panelState;
  const supportsSteering = panelState.supportsSteering;
  const hasContextComments =
    panelState.planComments.length > 0 ||
    panelState.pendingPRFeedback.length > 0 ||
    panelState.walkthroughComments.length > 0 ||
    panelState.messageComments.length > 0;
  return {
    onSubmit: handleSubmit,
    sessionId: resolvedSessionId,
    taskId,
    workspaceId: composerWorkspaceId,
    entityReferencesEnabled: true as const,
    taskTitle: panelState.task?.title,
    taskDescription: panelState.taskDescription ?? "",
    planModeEnabled,
    planModeAvailable: panelState.planModeAvailable,
    mcpServers: panelState.mcpServers,
    mcpAttachmentHistory: panelState.mcpAttachmentHistory,
    onPlanModeChange: panelState.handlePlanModeChange,
    isAgentBusy,
    supportsSteering,
    isStarting: panelState.isStarting,
    isPreparingEnvironment: panelState.isPreparingEnvironment,
    isMoving,
    isSending,
    onCancel: handleCancelTurn,
    placeholder,
    pendingClarification: panelState.pendingClarification,
    onClarificationResolved,
    showRequestChangesTooltip,
    onRequestChangesTooltipDismiss,
    pendingCommentsByFile: panelState.pendingCommentsByFile,
    hasContextComments,
    submitKey: panelState.chatSubmitKey,
    hasAgentCommands: !!(panelState.agentCommands && panelState.agentCommands.length > 0),
    isFailed: panelState.isFailed,
    isCompleted: panelState.isCompleted,
    sessionErrorMessage: panelState.session?.error_message,
    needsRecovery,
    executorUnavailable: executor.unavailable,
    executorUnavailableReason: executor.reason,
    contextItems: panelState.contextItems,
    planContextEnabled: panelState.planContextEnabled,
    contextFiles: panelState.contextFiles,
    onToggleContextFile: panelState.handleToggleContextFile,
    onAddContextFile: panelState.handleAddContextFile,
    onImplementPlan: implementPlanHandler,
    hideSessionsDropdown,
    minimalToolbar,
    hideAgentControls,
    hidePlanMode,
  };
}
