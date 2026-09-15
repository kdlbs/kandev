"use client";

import { useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { type Message } from "@/lib/types/http";
import { buildSessionCommands } from "@/components/session-commands";
import { shouldShowCancelAgent } from "@/components/task/chat/chat-input-container";

type QuickChatCancelCommandsProps = {
  sessionId: string;
  isWorking: boolean;
  pendingClarification?: Message | null;
  onCancel: () => Promise<void>;
};

/** Registers only the active structured Quick Chat conversation's cancel command. */
export function QuickChatCancelCommands({
  sessionId,
  isWorking,
  pendingClarification,
  onCancel,
}: QuickChatCancelCommandsProps) {
  const { t } = useTranslation();
  const storeApi = useAppStoreApi();
  const isActiveConversation = useAppStore(
    (state) =>
      state.quickChat.isOpen &&
      state.quickChat.activeKind === "conversation" &&
      state.quickChat.activeSessionId === sessionId,
  );
  const canCancel =
    isActiveConversation && shouldShowCancelAgent(isWorking, pendingClarification, sessionId);

  const cancelTurn = useCallback(async () => {
    const state = storeApi.getState();
    if (
      state.taskSessions.items[sessionId]?.cancellation_pending === true ||
      state.chatInput.cancellingBySessionId[sessionId] === true
    ) {
      return;
    }
    state.setCancelTurnPending(sessionId, true);
    try {
      await onCancel();
    } finally {
      state.setCancelTurnPending(sessionId, false);
    }
  }, [onCancel, sessionId, storeApi]);

  const commands = useMemo(
    () => buildSessionCommands(canCancel, cancelTurn, t),
    [canCancel, cancelTurn, t],
  );
  useRegisterCommands(commands);

  return null;
}
