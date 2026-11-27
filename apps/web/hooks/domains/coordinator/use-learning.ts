"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getDream,
  getLearning,
  getOutcomeMeasures,
  listDreams,
  putDreamRating,
  putShadowDream,
  type DreamDetail,
  type DreamRating,
  type DreamSummary,
  type LearningView,
  type OutcomeMeasures,
} from "@/lib/api/domains/coordinator-learning-api";

export type LoadStatus = "loading" | "ready" | "error";

type Loaded<T> = { status: LoadStatus; value: T | null; retry: () => void };

/** Runs one read on mount, on retry and when its inputs change; the latest read wins. */
function useRead<T>(read: () => Promise<T>, key: string): Loaded<T> {
  const [state, setState] = useState<{ status: LoadStatus; value: T | null }>({
    status: "loading",
    value: null,
  });
  const [tick, setTick] = useState(0);
  const readRef = useRef(read);
  readRef.current = read;
  useEffect(() => {
    let current = true;
    setState((prev) => ({ status: "loading", value: prev.value }));
    readRef
      .current()
      .then((value) => current && setState({ status: "ready", value }))
      .catch(() => current && setState((prev) => ({ status: "error", value: prev.value })));
    return () => {
      current = false;
    };
  }, [key, tick]);
  const retry = useCallback(() => setTick((n) => n + 1), []);
  return { status: state.status, value: state.value, retry };
}

export function useLearning(workspaceId: string, coordinatorId: string) {
  const loaded = useRead<LearningView>(
    () => getLearning(workspaceId, coordinatorId),
    `${workspaceId}/${coordinatorId}`,
  );
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  const [override, setOverride] = useState<LearningView | null>(null);
  const setShadowDream = useCallback(
    async (on: boolean) => {
      setPending(true);
      setFailed(false);
      try {
        setOverride(await putShadowDream(workspaceId, coordinatorId, on));
      } catch {
        setFailed(true);
      } finally {
        setPending(false);
      }
    },
    [workspaceId, coordinatorId],
  );
  return { ...loaded, value: override ?? loaded.value, setShadowDream, pending, failed };
}

export function useOutcomeMeasures(workspaceId: string, coordinatorId: string, days: number) {
  return useRead<OutcomeMeasures>(
    () => getOutcomeMeasures(workspaceId, coordinatorId, days),
    `${workspaceId}/${coordinatorId}/${days}`,
  );
}

/** The reports list: the first page, then older pages appended on demand. */
export function useDreamList(workspaceId: string, coordinatorId: string) {
  const first = useRead(
    () => listDreams(workspaceId, coordinatorId),
    `${workspaceId}/${coordinatorId}`,
  );
  const [older, setOlder] = useState<DreamSummary[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [moreFailed, setMoreFailed] = useState(false);
  const firstPage = first.value;
  const next = cursor ?? firstPage?.next_before ?? null;
  const loadMore = useCallback(async () => {
    if (!next) return;
    setMoreFailed(false);
    try {
      const page = await listDreams(workspaceId, coordinatorId, next);
      setOlder((prev) => [...prev, ...page.dreams]);
      setCursor(page.next_before ?? "");
    } catch {
      setMoreFailed(true);
    }
  }, [workspaceId, coordinatorId, next]);
  return {
    status: first.status,
    retry: first.retry,
    dreams: [...(firstPage?.dreams ?? []), ...older],
    hasMore: Boolean(next),
    loadMore,
    moreFailed,
  };
}

export function useDreamDetail(workspaceId: string, coordinatorId: string, dreamId: string) {
  const loaded = useRead<DreamDetail>(
    () => getDream(workspaceId, coordinatorId, dreamId),
    `${workspaceId}/${coordinatorId}/${dreamId}`,
  );
  const [ratings, setRatings] = useState<Record<string, DreamRating>>({});
  const [failedItem, setFailedItem] = useState<string | null>(null);
  const rate = useCallback(
    async (itemId: string, rating: DreamRating) => {
      setFailedItem(null);
      try {
        await putDreamRating({ workspaceId, coordinatorId, dreamId, itemId }, rating);
        setRatings((prev) => ({ ...prev, [itemId]: rating }));
      } catch {
        setFailedItem(itemId);
      }
    },
    [workspaceId, coordinatorId, dreamId],
  );
  return { ...loaded, ratings, rate, failedItem };
}
