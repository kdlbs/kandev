import { useCallback, useEffect, useMemo, useRef, useState } from "react";
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
import {
  selectActiveWorkloads,
  selectHistoryWorkloads,
  selectRunningWorkloads,
} from "@/lib/background-work/workload-views";

const EMPTY_WORKLOADS: WorkloadRunObservation[] = [];

type WorkloadCountSummary = {
  runningCount: number;
  waitingCount: number;
  unknownCount: number;
  totalCount: number;
  totalJobCount: number;
  hasActiveWork: boolean;
  hasHistory: boolean;
};

/** Counts the shared workload list once for the chip summary and history. */
function summarizeWorkloads(workloads: WorkloadRunObservation[]): WorkloadCountSummary {
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
    unknownCount: unknown,
    totalCount: active,
    totalJobCount: workloads.length,
    hasActiveWork: active > 0,
    hasHistory: workloads.length > 0,
  };
}

/** Derived counts and tab membership for the shared workload list. */
function useWorkloadViews(workloads: WorkloadRunObservation[]) {
  const summary = useMemo(() => summarizeWorkloads(workloads), [workloads]);
  const views = useMemo(
    () => ({
      runningWorkloads: selectRunningWorkloads(workloads),
      activeWorkloads: selectActiveWorkloads(workloads),
      historyWorkloads: selectHistoryWorkloads(workloads),
    }),
    [workloads],
  );
  return { ...summary, ...views };
}

export interface UseBackgroundWorkReturn {
  /** Every accepted observation, including terminal and unknown history. */
  workloads: WorkloadRunObservation[];
  /** Running records only; the composer chip and summary read this list. */
  runningWorkloads: WorkloadRunObservation[];
  /** Running and waiting records; the overview Active tab reads this list. */
  activeWorkloads: WorkloadRunObservation[];
  /** Terminal and unknown records; the overview History tab reads this list. */
  historyWorkloads: WorkloadRunObservation[];
  activeWorkload: WorkloadRunObservation | null;
  activeWorkId: string | null;
  runningCount: number;
  waitingCount: number;
  unknownCount: number;
  /** Active (running, waiting, or unknown) jobs; zero when only history remains. */
  totalCount: number;
  /** Every observed job, including completed and failed history. */
  totalJobCount: number;
  hasActiveWork: boolean;
  hasHistory: boolean;
  isLoading: boolean;
  /** True until this session's first read settles, so a fresh empty store does
   *  not resolve the overview before its rows arrive. */
  isInitialLoading: boolean;
  /** True when the last read failed and no verified rows are available. */
  loadError: boolean;
  executeAction: (
    req: Omit<BackgroundWorkActionRequest, "work_id"> & { work_id?: string },
  ) => Promise<BackgroundWorkActionResponse>;
  setActiveWorkload: (workId: string) => void;
  fetchWorkloads: () => Promise<void>;
}

/** The workload a session-level action targets: the explicit active id when
 *  present, otherwise the first accepted row. */
function useActiveWorkload(
  workloads: WorkloadRunObservation[],
  activeWorkId: string | null,
): WorkloadRunObservation | null {
  return useMemo(() => {
    if (!activeWorkId) return workloads[0] ?? null;
    return workloads.find((w) => w.work_id === activeWorkId) ?? workloads[0] ?? null;
  }, [workloads, activeWorkId]);
}

/** Session-scoped first-read readiness, so an empty initial store does not
 *  resolve the overview before its rows arrive. */
function useInitialWorkloadReadiness(
  sessionId: string | null,
  enabled: boolean,
  detailActive: boolean,
) {
  const sessionRef = useRef<string | null>(null);
  const [settledSessionId, setSettledSessionId] = useState<string | null>(null);
  const [failedSessionId, setFailedSessionId] = useState<string | null>(null);

  useEffect(() => {
    if (sessionRef.current === sessionId) return;
    sessionRef.current = sessionId;
    setSettledSessionId(null);
    setFailedSessionId(null);
  }, [sessionId]);

  const settle = useCallback((target: string, failed: boolean) => {
    if (sessionRef.current !== target) return;
    setSettledSessionId(target);
    setFailedSessionId(failed ? target : null);
  }, []);

  const isInitialLoading =
    Boolean(detailActive && enabled && sessionId) &&
    (sessionRef.current !== sessionId || settledSessionId !== sessionId);
  const loadError = Boolean(sessionId) && failedSessionId === sessionId;

  return { isInitialLoading, loadError, settle };
}

/** Read the session-scoped store projection. */
function useSessionBackgroundWorkState(sessionId: string | null, enabled: boolean) {
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
  return { workloads, activeWorkId, isLoading };
}

/** An HTTP snapshot can omit observations accepted after its request began. */
function preserveLiveWorkloads(
  snapshot: WorkloadRunObservation[],
  baseline: WorkloadRunObservation[],
  current: WorkloadRunObservation[],
): WorkloadRunObservation[] {
  const beforeRead = new Map(baseline.map((work) => [work.work_id, work]));
  const snapshotIds = new Set(snapshot.map((work) => work.work_id));
  return [
    ...snapshot,
    ...current.filter(
      (work) => !snapshotIds.has(work.work_id) && beforeRead.get(work.work_id) !== work,
    ),
  ];
}

async function readBackgroundWorkloads(
  store: ReturnType<typeof useAppStoreApi>,
  sessionId: string,
): Promise<boolean> {
  const generation = store.getState().beginBackgroundWorkRead(sessionId);
  const baseline =
    store.getState().backgroundWork.workloadsBySessionId[sessionId] ?? EMPTY_WORKLOADS;
  const ownsRead = () =>
    store.getState().backgroundWork.readGenerationBySessionId[sessionId] === generation;
  try {
    const response = await listBackgroundWorkloads(sessionId);
    if (!ownsRead()) return false;
    const current =
      store.getState().backgroundWork.workloadsBySessionId[sessionId] ?? EMPTY_WORKLOADS;
    store
      .getState()
      .setBackgroundWorkloads(
        sessionId,
        preserveLiveWorkloads(response.workloads ?? [], baseline, current),
      );
    return false;
  } catch {
    if (!ownsRead()) return false;
    store.getState().setBackgroundWorkLoading(sessionId, false);
    return true;
  }
}

export function useBackgroundWork(
  sessionId: string | null,
  { detailActive = true }: { detailActive?: boolean } = {},
): UseBackgroundWorkReturn {
  const store = useAppStoreApi();
  const enabled = useFeature("agentBackgroundWork");
  const { workloads, activeWorkId, isLoading } = useSessionBackgroundWorkState(sessionId, enabled);

  const lastFetchedSessionIdRef = useRef<string | null>(null);
  const latestFetchIdRef = useRef(0);
  const { isInitialLoading, loadError, settle } = useInitialWorkloadReadiness(
    sessionId,
    enabled,
    detailActive,
  );

  const fetchWorkloads = useCallback(async () => {
    if (!detailActive || !sessionId || !enabled) return;
    const requestId = (latestFetchIdRef.current += 1);
    const failed = await readBackgroundWorkloads(store, sessionId);
    if (requestId === latestFetchIdRef.current) settle(sessionId, failed);
  }, [detailActive, sessionId, enabled, store, settle]);

  useEffect(() => {
    if (!detailActive || !sessionId || !enabled) {
      lastFetchedSessionIdRef.current = null;
      return;
    }
    if (lastFetchedSessionIdRef.current === sessionId) return;
    lastFetchedSessionIdRef.current = sessionId;
    fetchWorkloads();
  }, [detailActive, sessionId, enabled, fetchWorkloads]);

  const activeWorkload = useActiveWorkload(workloads, activeWorkId);

  const {
    runningCount,
    waitingCount,
    unknownCount,
    totalCount,
    totalJobCount,
    hasActiveWork,
    hasHistory,
    runningWorkloads,
    activeWorkloads,
    historyWorkloads,
  } = useWorkloadViews(workloads);

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
    runningWorkloads,
    activeWorkloads,
    historyWorkloads,
    activeWorkload,
    activeWorkId,
    runningCount,
    waitingCount,
    unknownCount,
    totalCount,
    totalJobCount,
    hasActiveWork,
    hasHistory,
    isLoading,
    isInitialLoading: isInitialLoading || (isLoading && workloads.length === 0),
    loadError,
    executeAction,
    setActiveWorkload,
    fetchWorkloads,
  };
}
