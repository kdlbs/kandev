"use client";

import { usePromptSuggestion } from "@/hooks/use-prompt-suggestion";
import { hasPendingClarification } from "./types";
import type { ChatPanelState } from "./use-chat-panel-state";

/** Props the shared composer needs to show and dismiss a next-prompt suggestion. */
export function useComposerPromptSuggestion(panelState: ChatPanelState, isMoving: boolean) {
  const clarificationPending = hasPendingClarification(
    Boolean(panelState.pendingClarification),
    panelState.session?.pending_action,
  );
  const { suggestion, dismiss } = usePromptSuggestion({
    sessionId: panelState.resolvedSessionId,
    taskTitle: panelState.task?.title ?? "",
    blocked: panelState.isAgentBusy || panelState.needsRecovery || clarificationPending || isMoving,
  });
  return { promptSuggestion: suggestion, onPromptSuggestionDismiss: dismiss };
}
