"use client";

import type {
  ChatSubmitPayload,
  ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import type { ChatPanelState } from "./use-chat-panel-state";
import { readAgentDeliveryRecovery } from "@/lib/session-agent-delivery-recovery";

// i18n-exempt: stable backend failure code, not user-facing copy.
const DURABLE_DELIVERY_UNCERTAIN = "DURABLE_DELIVERY_UNCERTAIN";

type ComposerPropsArgs = {
  panelState: ChatPanelState;
  composerWorkspaceId: string | null;
  workspaceResolutionFailed: boolean;
  onRetryWorkspaceResolution: () => void;
  isMoving: boolean;
  implementPlanHandler: ((fresh: boolean) => Promise<void> | Promise<boolean>) | undefined;
  executor: { unavailable: boolean; reason?: string };
  placeholder: string;
  handleSubmit: (payload: ChatSubmitPayload) => ChatSubmitResult;
  handleCancelTurn: () => Promise<void>;
  isSending: boolean;
  launchErrorOwned?: boolean;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  onClarificationResolved: () => void;
  hideSessionsDropdown?: boolean;
  minimalToolbar?: boolean;
  hideAgentControls?: boolean;
  hidePlanMode?: boolean;
};

type DeliveryRecoveryPresentation = {
  uncertainDelivery: boolean;
  deliveryRecoveryPhase: "reconnecting" | "uncertain" | undefined;
};

function deliveryRecoveryPresentation(
  phase: NonNullable<ReturnType<typeof readAgentDeliveryRecovery>>["phase"] | undefined,
  legacyUncertainDelivery: boolean,
): DeliveryRecoveryPresentation {
  if (phase === "reconnecting" || phase === "uncertain") {
    return { uncertainDelivery: true, deliveryRecoveryPhase: phase };
  }
  if (phase) return { uncertainDelivery: false, deliveryRecoveryPhase: undefined };
  return {
    uncertainDelivery: legacyUncertainDelivery,
    deliveryRecoveryPhase: legacyUncertainDelivery ? "uncertain" : undefined,
  };
}

function hasContextComments(panelState: ChatPanelState): boolean {
  return (
    panelState.planComments.length > 0 ||
    (panelState.previewFeedback?.length ?? 0) > 0 ||
    panelState.pendingPRFeedback.length > 0 ||
    panelState.walkthroughComments.length > 0 ||
    panelState.messageComments.length > 0
  );
}

/**
 * Builds the (large) prop object for `ChatInputContainer` from panel state
 * and derived values. Kept separate so `ChatInputArea`'s render body stays
 * short — behavior is identical to inlining every prop.
 */
export function useComposerProps(args: ComposerPropsArgs) {
  const {
    panelState,
    composerWorkspaceId,
    workspaceResolutionFailed,
    onRetryWorkspaceResolution,
    isMoving,
    implementPlanHandler,
    executor,
    placeholder,
    handleSubmit,
    handleCancelTurn,
    isSending,
    launchErrorOwned,
    showRequestChangesTooltip,
    onRequestChangesTooltipDismiss,
    onClarificationResolved,
    hideSessionsDropdown,
    minimalToolbar,
    hideAgentControls,
    hidePlanMode,
  } = args;
  const { resolvedSessionId, taskId, isAgentBusy, isWorking, needsRecovery, planModeEnabled } =
    panelState;
  const deliveryRecovery = readAgentDeliveryRecovery(panelState.session?.metadata);
  const legacyUncertainDelivery = panelState.lastAgentError?.code === DURABLE_DELIVERY_UNCERTAIN;
  const recoveryPresentation = deliveryRecoveryPresentation(
    deliveryRecovery?.phase,
    legacyUncertainDelivery,
  );
  const canQueueWhileStarting = panelState.inputMode === "queue" && panelState.isQueueReady;
  const supportsSteering = panelState.supportsSteering;
  const hasPendingContextComments = hasContextComments(panelState);
  return {
    onSubmit: handleSubmit,
    sessionId: resolvedSessionId,
    taskId,
    workspaceId: composerWorkspaceId,
    workspaceResolutionFailed,
    onRetryWorkspaceResolution,
    entityReferencesEnabled: true as const,
    taskTitle: panelState.task?.title,
    taskDescription: panelState.taskDescription ?? "",
    planModeEnabled,
    planModeAvailable: panelState.planModeAvailable,
    mcpServers: panelState.mcpServers,
    mcpAttachmentHistory: panelState.mcpAttachmentHistory,
    onPlanModeChange: panelState.handlePlanModeChange,
    isAgentBusy,
    isWorking,
    supportsSteering,
    isStarting: panelState.isStarting,
    canQueueWhileStarting,
    isPreparingEnvironment: panelState.isPreparingEnvironment,
    isMoving,
    isSending,
    launchErrorOwned,
    onCancel: handleCancelTurn,
    placeholder,
    pendingClarification: panelState.pendingClarification,
    onClarificationResolved,
    showRequestChangesTooltip,
    onRequestChangesTooltipDismiss,
    pendingCommentsByFile: panelState.pendingCommentsByFile,
    hasContextComments: hasPendingContextComments,
    submitKey: panelState.chatSubmitKey,
    hasAgentCommands: !!(panelState.agentCommands && panelState.agentCommands.length > 0),
    isFailed: panelState.isFailed,
    isCompleted: panelState.isCompleted,
    sessionErrorMessage: panelState.session?.error_message,
    ...recoveryPresentation,
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
