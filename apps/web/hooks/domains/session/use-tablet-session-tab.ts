import { useCallback, useLayoutEffect, useMemo, type SetStateAction } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getActiveTabForSession, setActiveTabForSession } from "@/lib/local-storage";
import { hasTaskRouteFocus, consumeTaskRouteFocus } from "@/lib/state/task-navigation-focus";

/** Tablet tab selection is shared with unread display, not the phone panel preference. */
export function useTabletSessionTab(sessionId: string | null) {
  const store = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const revision = useAppStore((state) => state.taskRemoval.navigationRevision);
  const initialTab = useMemo(() => {
    if (typeof window !== "undefined" && sessionId) {
      const saved = getActiveTabForSession(sessionId, "chat");
      if (saved === "chat" || saved === "changes" || saved === "pr" || saved.startsWith("file:"))
        return saved;
    }
    return "chat";
  }, [sessionId]);
  const tab = useAppStore((state) =>
    sessionId ? (state.tabletSession.activeTabBySessionId[sessionId] ?? initialTab) : "chat",
  );
  const setTab = useCallback(
    (next: SetStateAction<string>) => {
      if (!sessionId) return;
      const state = store.getState();
      const previous = state.tabletSession.activeTabBySessionId[sessionId] ?? initialTab;
      const selected = typeof next === "function" ? next(previous) : next;
      state.setTabletSessionTab(sessionId, selected);
      setActiveTabForSession(sessionId, selected);
    },
    [store, sessionId, initialTab],
  );
  useLayoutEffect(() => {
    if (sessionId && store.getState().tabletSession.activeTabBySessionId[sessionId] === undefined)
      setTab(initialTab);
  }, [store, sessionId, initialTab, setTab]);
  useLayoutEffect(() => {
    if (!sessionId || !hasTaskRouteFocus(store, taskId, sessionId)) return;
    setTab("chat");
    const state = store.getState();
    consumeTaskRouteFocus(
      store,
      taskId,
      sessionId,
      state.environmentIdBySessionId[sessionId] ??
        state.taskSessions.items[sessionId]?.task_environment_id ??
        null,
    );
  }, [store, taskId, sessionId, revision, setTab]);
  return [tab, setTab] as const;
}
