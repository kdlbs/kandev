import { useCallback, useEffect, useMemo, useRef } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import {
  listBackgroundWorkloads,
  executeBackgroundAction,
} from "@/lib/api/domains/background-work-api";
import type {
  WorkloadRunObservation,
  BackgroundWorkActionRequest,
  BackgroundWorkActionResponse,
} from "@/lib/types/background-work";

const EMPTY_WORKLOADS: WorkloadRunObservation[] = [];

export interface UseBackgroundWorkReturn {
  workloads: WorkloadRunObservation[];
  activeWorkload: WorkloadRunObservation | null;
  activeWorkId: string | null;
  runningCount: number;
  waitingCount: number;
  totalCount: number;
  hasActiveWork: boolean;
  isLoading: boolean;
  executeAction: (
    req: Omit<BackgroundWorkActionRequest, "work_id"> & { work_id?: string },
  ) => Promise<BackgroundWorkActionResponse>;
  setActiveWorkload: (workId: string) => void;
  fetchWorkloads: () => Promise<void>;
}

export function useBackgroundWork(sessionId: string | null): UseBackgroundWorkReturn {
  const store = useAppStoreApi();
  const enabled = useFeature("agentBackgroundWork");

  const workloads = useAppStore((state) => {
    if (!sessionId || !enabled) return EMPTY_WORKLOADS;
    return state.backgroundWork.workloadsBySessionId[sessionId] ?? EMPTY_WORKLOADS;
  });

  const activeWorkId = useAppStore((state) => {
    if (!sessionId || !enabled) return null;
    return state.backgroundWork.activeWorkIdBySessionId[sessionId] ?? null;
  });

  const isLoading = useAppStore((state) => {
    if (!sessionId || !enabled) return false;
    return state.backgroundWork.loadingBySessionId[sessionId] ?? false;
  });

  const lastFetchedSessionIdRef = useRef<string | null>(null);

  const fetchWorkloads = useCallback(async () => {
    if (!sessionId || !enabled) return;
    try {
      store.getState().setBackgroundWorkLoading?.(sessionId, true);
      const response = await listBackgroundWorkloads(sessionId);
      store.getState().setBackgroundWorkloads(sessionId, response.workloads ?? []);
    } catch {
      store.getState().setBackgroundWorkLoading?.(sessionId, false);
    }
  }, [sessionId, enabled, store]);

  useEffect(() => {
    if (!sessionId || !enabled) {
      lastFetchedSessionIdRef.current = null;
      return;
    }
    if (lastFetchedSessionIdRef.current === sessionId) return;
    lastFetchedSessionIdRef.current = sessionId;
    fetchWorkloads();
  }, [sessionId, enabled, fetchWorkloads]);

  const activeWorkload = useMemo(() => {
    if (!activeWorkId) return workloads[0] ?? null;
    return workloads.find((w) => w.work_id === activeWorkId) ?? workloads[0] ?? null;
  }, [workloads, activeWorkId]);

  const { runningCount, waitingCount, totalCount, hasActiveWork } = useMemo(() => {
    let running = 0;
    let waiting = 0;
    let unknown = 0;
    for (const w of workloads) {
      if (w.state === "running") running++;
      else if (w.state === "waiting") waiting++;
      else if (w.state === "unknown") unknown++;
    }
    const active = running + waiting + unknown;
    return {
      runningCount: running,
      waitingCount: waiting,
      totalCount: active,
      hasActiveWork: active > 0,
    };
  }, [workloads]);

  const executeAction = useCallback(
    async (
      req: Omit<BackgroundWorkActionRequest, "work_id"> & { work_id?: string },
    ): Promise<BackgroundWorkActionResponse> => {
      const targetWorkId = req.work_id || activeWorkId || (workloads[0]?.work_id ?? "");
      if (!sessionId || !targetWorkId) {
        return {
          success: false,
          work_id: targetWorkId,
          action: req.action,
          error: "No active background work target",
        };
      }
      return executeBackgroundAction(sessionId, {
        ...req,
        work_id: targetWorkId,
      });
    },
    [sessionId, activeWorkId, workloads],
  );

  const setActiveWorkload = useCallback(
    (workId: string) => {
      if (!sessionId) return;
      store.getState().setActiveBackgroundWorkload(sessionId, workId);
    },
    [sessionId, store],
  );

  return {
    workloads,
    activeWorkload,
    activeWorkId,
    runningCount,
    waitingCount,
    totalCount,
    hasActiveWork,
    isLoading,
    executeAction,
    setActiveWorkload,
    fetchWorkloads,
  };
}
