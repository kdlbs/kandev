import { useCallback, useLayoutEffect, useMemo, type SetStateAction } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { getActiveTabForSession, setActiveTabForSession } from "@/lib/local-storage";
import { hasTaskRouteFocus, consumeTaskRouteFocus } from "@/lib/state/task-navigation-focus";

import { beginTabletSelection, hasPendingTabletFileOpen } from "@/lib/state/tablet-file-authority";

/** Tablet tab selection is shared with unread display, not the phone panel preference. */
export function useTabletSessionTab(sessionId: string | null) {
  const store = useAppStoreApi();
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const revision = useAppStore((state) => state.taskRemoval.navigationRevision);
  const workflowRequest = useAppStore((state) => state.workflowSessionFocus.request);
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
      if (selected !== previous || hasPendingTabletFileOpen(store, sessionId)) {
        beginTabletSelection(store, sessionId);
        if (selected !== previous) state.setTabletSessionTab(sessionId, selected);
      }
      setActiveTabForSession(sessionId, selected);
    },
    [store, sessionId, initialTab],
  );
  useLayoutEffect(() => {
    if (sessionId && store.getState().tabletSession.activeTabBySessionId[sessionId] === undefined)
      store.getState().setTabletSessionTab(sessionId, initialTab);
  }, [store, sessionId, initialTab]);
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
  useLayoutEffect(() => {
    const state = store.getState();
    if (
      !sessionId ||
      !workflowRequest ||
      state.workflowSessionFocus.request !== workflowRequest ||
      workflowRequest.taskId !== taskId ||
      workflowRequest.sessionId !== sessionId ||
      state.tasks.activeTaskId !== taskId ||
      state.tasks.activeSessionId !== sessionId
    )
      return;
    setTab("chat");
    state.acknowledgeWorkflowSessionFocus(workflowRequest.requestId);
  }, [store, taskId, sessionId, workflowRequest, setTab]);
  return [tab, setTab] as const;
}
