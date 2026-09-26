"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { getTaskQuorum } from "@/lib/api/domains/office-extended-api";
import type { QuorumResponseDTO } from "@/lib/state/slices/office/quorum-types";
import { t } from "@/lib/i18n";

export type UseTaskQuorumResult = {
  quorum: QuorumResponseDTO | undefined;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
};

export function useTaskQuorum(
  taskId: string | null,
  workspaceId: string | null,
): UseTaskQuorumResult {
  const quorum = useAppStore((s) => (taskId ? s.office.taskQuorum.byTaskId[taskId] : undefined));
  const setTaskQuorum = useAppStore((s) => s.setTaskQuorum);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetchedScope, setFetchedScope] = useState<string | null>(null);
  const requestGeneration = useRef(0);

  const refresh = useCallback(async () => {
    if (!taskId || !workspaceId) return;
    const generation = ++requestGeneration.current;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getTaskQuorum(taskId, workspaceId);
      if (generation !== requestGeneration.current) return;
      setTaskQuorum(taskId, res);
      setFetchedScope(`${workspaceId}:${taskId}`);
    } catch (e) {
      if (generation !== requestGeneration.current) return;
      setError(e instanceof Error ? e.message : t("office:failedToLoadTaskQuorum"));
    } finally {
      if (generation === requestGeneration.current) setIsLoading(false);
    }
  }, [taskId, workspaceId, setTaskQuorum]);

  useEffect(() => {
    requestGeneration.current += 1;
  }, [taskId, workspaceId]);

  useEffect(() => {
    if (!taskId || !workspaceId || fetchedScope === `${workspaceId}:${taskId}`) return;
    void refresh();
  }, [taskId, workspaceId, fetchedScope, refresh]);

  // office.task.decision_recorded (lib/ws/handlers/office.ts) fires
  // `task:${taskId}` — recording a decision changes the guard's
  // approve/reject count, so the cached snapshot must be invalidated even
  // though taskId itself hasn't changed.
  useOfficeRefetch(taskId ? `task:${taskId}` : "", () => {
    if (taskId && workspaceId) void refresh();
  });

  return { quorum, isLoading, error, refresh };
}
