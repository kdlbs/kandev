"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getTaskQuorum } from "@/lib/api/domains/office-extended-api";
import type { QuorumResponseDTO } from "@/lib/state/slices/office/quorum-types";
import { t } from "@/lib/i18n";

export type UseTaskQuorumResult = {
  quorum: QuorumResponseDTO | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

export function useTaskQuorum(taskId: string | null): UseTaskQuorumResult {
  const quorum = useAppStore((s) => (taskId ? s.office.taskQuorum.byTaskId[taskId] : undefined));
  const setTaskQuorum = useAppStore((s) => s.setTaskQuorum);
  // office.task.decision_recorded (lib/ws/handlers/office.ts) bumps this
  // counter for `task:${taskId}` — recording a decision changes the guard's
  // approve/reject count, so the cached snapshot must be invalidated even
  // though taskId itself hasn't changed.
  const decisionRefetchTrigger = useAppStore((s) =>
    taskId ? s.office.refetchTriggers[`task:${taskId}`] : undefined,
  );
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetchedTaskId, setFetchedTaskId] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!taskId) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getTaskQuorum(taskId);
      setTaskQuorum(taskId, res);
      setFetchedTaskId(taskId);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("office:failedToLoadTaskQuorum"));
    } finally {
      setIsLoading(false);
    }
  }, [taskId, setTaskQuorum]);

  useEffect(() => {
    if (!taskId || fetchedTaskId === taskId) return;
    void refresh();
  }, [taskId, fetchedTaskId, refresh]);

  useEffect(() => {
    if (!taskId || decisionRefetchTrigger === undefined) return;
    void refresh();
  }, [taskId, decisionRefetchTrigger, refresh]);

  return { quorum, isLoading, error, refresh };
}
