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
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetched, setFetched] = useState(false);

  const refresh = useCallback(async () => {
    if (!taskId) return;
    setIsLoading(true);
    setError(null);
    try {
      const res = await getTaskQuorum(taskId);
      setTaskQuorum(taskId, res);
      setFetched(true);
    } catch (e) {
      setError(e instanceof Error ? e.message : t("office:failedToLoadTaskQuorum"));
    } finally {
      setIsLoading(false);
    }
  }, [taskId, setTaskQuorum]);

  useEffect(() => {
    if (!taskId || fetched) return;
    void refresh();
  }, [taskId, fetched, refresh]);

  return { quorum, isLoading, error, refresh };
}
