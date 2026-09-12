import { useCallback, useEffect, useMemo, useRef } from "react";
import { useShallow } from "zustand/react/shallow";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import { planCommentRecovery } from "@/lib/plan-comment-recovery";
import { planCommentMigrationFor } from "./plan-comment-migration";

/** Losslessly promotes identified browser drafts, independently of ordinary comment reads. */
export function usePlanCommentMigration(taskId: string | null | undefined) {
  const store = useAppStoreApi();
  const { sessions, isLoaded, isLoading, error, loadSessions } = useTaskSessions(taskId ?? null);
  const knownSessionIds = useAppStore(
    useShallow((state) =>
      Object.values(state.taskSessions.items)
        .filter((session) => session.task_id === taskId)
        .map((session) => session.id),
    ),
  );
  const connected = useAppStore((state) => state.connection.status === "connected");
  const state = useAppStore((state) =>
    taskId ? state.taskPlans.commentsMigrationByTaskId[taskId] : undefined,
  );
  const recovery = useMemo(
    () => (taskId ? planCommentMigrationFor(store, taskId) : null),
    [store, taskId],
  );
  const discover = useRef(loadSessions);
  useEffect(() => {
    discover.current = loadSessions;
  }, [loadSessions]);
  useEffect(() => recovery?.attach(() => discover.current(true)), [recovery]);
  useEffect(() => {
    recovery?.update({
      sessionIds: [...new Set([...sessions.map((session) => session.id), ...knownSessionIds])],
      complete: isLoaded && !error,
      loading: isLoading,
    });
  }, [recovery, sessions, knownSessionIds, isLoaded, isLoading, error]);
  useForegroundRefresh(() => recovery?.wake(), Boolean(taskId) && connected, recovery);
  const retry = useCallback(async () => {
    await recovery?.retry();
  }, [recovery]);
  return { ...planCommentRecovery(state), retry };
}
