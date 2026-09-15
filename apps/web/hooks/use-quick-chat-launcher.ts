import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  captureQuickChatLauncherFocus,
  requestQuickChatClose,
  type QuickChatLauncherFocusOptions,
} from "@/components/quick-chat/quick-chat-focus";
import { orderQuickChatTabs } from "@/lib/state/slices/ui/quick-chat-tab-order";
import type { QuickChatSessionKind } from "@/lib/state/slices/ui/types";

type QuickChatLauncherOptions = Pick<QuickChatLauncherFocusOptions, "returnFocusRef"> & {
  silentFocusReturn?: boolean;
  toggleWhenOpen?: boolean;
};

/**
 * Hook to handle opening quick chat.
 * Just opens the modal - the user will select an agent from the picker.
 */
export function useQuickChatLauncher(
  workspaceId?: string | null,
  kind: QuickChatSessionKind = "chat",
  options: QuickChatLauncherOptions = {},
) {
  const silentFocusReturn = options.silentFocusReturn ?? true;
  const toggleWhenOpen = options.toggleWhenOpen ?? false;
  const returnFocusRef = options.returnFocusRef;
  const openQuickChat = useAppStore((state) => state.openQuickChat);
  const closeQuickChat = useAppStore((state) => state.closeQuickChat);
  const isQuickChatOpen = useAppStore((state) => state.quickChat.isOpen);
  const quickChatSessions = useAppStore((state) => state.quickChat.sessions);
  const activeSessionId = useAppStore((state) => state.quickChat.activeSessionId);
  const selectionRevision = useAppStore(
    (state) => state.quickChat.selectionRevisionByWorkspace[workspaceId ?? ""] ?? 0,
  );
  const rememberedSelection = useAppStore(
    (state) => state.quickChat.rememberedSelectionByWorkspace[workspaceId ?? ""],
  );
  const selectionReady = useAppStore(
    (state) => state.quickChat.selectionReadyByWorkspace[workspaceId ?? ""] ?? false,
  );
  const tabOrder = useAppStore((state) => {
    const workspace = workspaceId ?? "";
    return (
      state.quickChat.tabOrderByWorkspace[workspace] ??
      state.userSettings.quickChatTabOrderByWorkspace[workspace]
    );
  });
  const requestQuickChatOpen = useAppStore((state) => state.requestQuickChatOpen);

  const handleOpenQuickChat = useCallback(() => {
    if (!workspaceId) return;
    if (toggleWhenOpen && isQuickChatOpen) {
      if (!requestQuickChatClose()) closeQuickChat();
      return;
    }
    captureQuickChatLauncherFocus({ silent: silentFocusReturn, returnFocusRef });

    // If there's an existing session, open it. Otherwise just open the modal with agent picker
    const matchingSessions = quickChatSessions.filter(
      (session) => session.workspaceId === workspaceId && (session.kind ?? "chat") === kind,
    );
    const existingSession =
      (selectionRevision > 0
        ? matchingSessions.find((session) => session.sessionId === activeSessionId)
        : undefined) ??
      matchingSessions.find((session) => session.sessionId === rememberedSelection?.[kind]) ??
      (selectionReady ? orderQuickChatTabs(matchingSessions, [], tabOrder).sessions[0] : undefined);
    if (!selectionReady && !existingSession) {
      if (tabOrder) requestQuickChatOpen(workspaceId, kind, tabOrder);
      else requestQuickChatOpen(workspaceId, kind);
      return;
    }
    if (existingSession) {
      openQuickChat(
        existingSession.sessionId,
        workspaceId,
        undefined,
        existingSession.kind ?? "chat",
      );
    } else {
      // Open modal without a session - will show agent picker
      openQuickChat("", workspaceId, undefined, kind);
    }
  }, [
    workspaceId,
    toggleWhenOpen,
    isQuickChatOpen,
    closeQuickChat,
    silentFocusReturn,
    returnFocusRef,
    quickChatSessions,
    kind,
    activeSessionId,
    selectionRevision,
    rememberedSelection,
    selectionReady,
    tabOrder,
    requestQuickChatOpen,
    openQuickChat,
  ]);

  return handleOpenQuickChat;
}
