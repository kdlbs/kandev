"use client";

import { useCallback, useMemo, useRef } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createQueuedUserSettingsSyncWithResponse } from "@/lib/user-settings-sync";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { orderQuickChatTabs } from "@/lib/state/slices/ui/quick-chat-tab-order";
import type { QuickChatSession, QuickTerminalTab } from "@/lib/state/slices/ui/types";

export const conversationTabReference = (sessionId: string) => `conversation:${sessionId}`;
export const terminalTabReference = (tabId: string) => `terminal:${tabId}`;

export function adjacentQuickChatTabReference(
  order: readonly string[],
  reference: string,
): string | undefined {
  const index = order.indexOf(reference);
  if (index < 0) return undefined;
  return order[index + 1] ?? order[index - 1];
}

type QuickChatTabOrderActions = {
  setQuickChatTabOrder: (workspaceId: string, order: string[]) => void;
  clearQuickChatTabOrder: (workspaceId: string, expectedOrder: string[]) => void;
  setQuickChatTabOrderSyncState: (
    workspaceId: string,
    state: { pending: boolean; error: string | null },
  ) => void;
};

const syncQuickChatTabOrder = createQueuedUserSettingsSyncWithResponse<Record<string, string[]>>(
  (orderByWorkspace) => ({
    quick_chat_tab_order_by_workspace: orderByWorkspace,
  }),
);

function cloneTabOrderMap(source: Record<string, string[]> | undefined) {
  return Object.fromEntries(
    Object.entries(source ?? {}).map(([workspaceId, order]) => [workspaceId, [...order]]),
  );
}

/** Provides mixed-tab ordering and a serialized user-settings save queue. */
export function useQuickChatTabOrder(
  workspaceId: string,
  sessions: QuickChatSession[],
  terminalTabs: QuickTerminalTab[],
) {
  const appStore = useAppStoreApi();
  const state = useAppStore(
    useShallow((store) => ({
      persistedOrder: store.userSettings.quickChatTabOrderByWorkspace[workspaceId],
      optimisticOrder: store.quickChat.tabOrderByWorkspace[workspaceId],
      syncError: store.quickChat.tabOrderSyncErrorByWorkspace[workspaceId] ?? null,
      syncPending: store.quickChat.tabOrderSyncPendingByWorkspace[workspaceId] ?? false,
      setQuickChatTabOrder: store.setQuickChatTabOrder,
      clearQuickChatTabOrder: store.clearQuickChatTabOrder,
      setQuickChatTabOrderSyncState: store.setQuickChatTabOrderSyncState,
    })),
  ) as {
    persistedOrder: string[] | undefined;
    optimisticOrder: string[] | undefined;
    syncError: string | null;
    syncPending: boolean;
  } & QuickChatTabOrderActions;
  const saveVersion = useRef(0);

  const ordered = useMemo(() => {
    return orderQuickChatTabs(
      sessions,
      terminalTabs,
      state.optimisticOrder ?? state.persistedOrder,
    );
  }, [sessions, state.optimisticOrder, state.persistedOrder, terminalTabs]);

  const persistOrder = useCallback(
    (order: string[]) => {
      const nextOrder = [...order];
      const current = appStore.getState();
      const nextMap = cloneTabOrderMap(current.userSettings.quickChatTabOrderByWorkspace);
      nextMap[workspaceId] = nextOrder;
      state.setQuickChatTabOrder(workspaceId, nextOrder);
      state.setQuickChatTabOrderSyncState(workspaceId, { pending: true, error: null });

      const version = ++saveVersion.current;
      void syncQuickChatTabOrder(nextMap)
        .then((response) => {
          const currentState = appStore.getState();
          currentState.setUserSettings(
            mapUserSettingsResponse(response, currentState.userSettings),
          );
          if (version === saveVersion.current) {
            state.clearQuickChatTabOrder(workspaceId, nextOrder);
            state.setQuickChatTabOrderSyncState(workspaceId, { pending: false, error: null });
          }
        })
        .catch((error: unknown) => {
          if (version !== saveVersion.current) return;
          state.setQuickChatTabOrderSyncState(workspaceId, {
            pending: false,
            error: error instanceof Error ? error.message : String(error),
          });
        });
    },
    [appStore, state, workspaceId],
  );

  const removeTabReference = useCallback(
    (reference: string) => {
      const current = appStore.getState();
      const currentOrder =
        current.quickChat.tabOrderByWorkspace[workspaceId] ??
        current.userSettings.quickChatTabOrderByWorkspace[workspaceId] ??
        ordered.order;
      persistOrder(currentOrder.filter((item) => item !== reference));
    },
    [appStore, ordered.order, persistOrder, workspaceId],
  );

  return {
    ...ordered,
    persistOrder,
    removeTabReference,
    syncError: state.syncError,
    syncPending: state.syncPending,
  };
}
