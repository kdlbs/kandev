"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getTurnChangeHistoryItem,
  listTurnChangeHistory,
} from "@/lib/api/domains/turn-changes-api";
import type { TurnChangeHistoryPage, TurnChangeSetSummary } from "@/lib/types/turn-changes";

const PAGE_SIZE = 50;
const EMPTY_SUMMARIES: TurnChangeSetSummary[] = [];
const inFlightPages = new Map<string, Promise<TurnChangeHistoryPage>>();

type TurnChangeSummaryState = {
  bySession: Record<string, TurnChangeSetSummary[] | undefined>;
};

export function historyPageCursor(page: TurnChangeHistoryPage): {
  offset: number;
  nextOffset?: number;
} {
  return { offset: page.offset, nextOffset: page.next_offset };
}

export function selectTurnChangeSummaries(
  state: TurnChangeSummaryState,
  sessionId: string | null,
): TurnChangeSetSummary[] {
  return sessionId ? (state.bySession[sessionId] ?? EMPTY_SUMMARIES) : EMPTY_SUMMARIES;
}

async function loadPage(
  sessionId: string,
  offset: number,
  store: ReturnType<typeof useAppStoreApi>,
): Promise<TurnChangeHistoryPage> {
  const key = `${sessionId}:${offset}`;
  const active = inFlightPages.get(key);
  if (active) return active;
  const request = listTurnChangeHistory(sessionId, offset, PAGE_SIZE, { cache: "no-store" });
  inFlightPages.set(key, request);
  try {
    const page = await request;
    if (!store.getState().taskSessions.items[sessionId]) return page;
    store.getState().mergeTurnChangePage(sessionId, page.change_sets, historyPageCursor(page));
    return page;
  } finally {
    inFlightPages.delete(key);
  }
}

export function clearTurnChangeRequestsForTest(): void {
  inFlightPages.clear();
}

export function useSessionTurnChanges(sessionId: string | null) {
  const store = useAppStoreApi();
  const summaries = useAppStore((state) => selectTurnChangeSummaries(state.turnChanges, sessionId));
  const loaded = useAppStore((state) =>
    sessionId ? (state.turnChanges.loadedBySession[sessionId] ?? false) : false,
  );
  const nextOffset = useAppStore((state) =>
    sessionId ? (state.turnChanges.nextOffsetBySession[sessionId] ?? null) : null,
  );
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(
    async (offset: number) => {
      if (!sessionId) return;
      setLoading(true);
      setError(null);
      try {
        await loadPage(sessionId, offset, store);
      } catch (err) {
        setError(err);
      } finally {
        setLoading(false);
      }
    },
    [sessionId, store],
  );

  useEffect(() => {
    if (sessionId && !loaded) void load(0);
  }, [sessionId, loaded, load]);

  const loadMore = useCallback(() => {
    if (nextOffset !== null) void load(nextOffset);
  }, [load, nextOffset]);

  const ensureTurnsLoaded = useCallback(
    async (turnIds: string[]) => {
      if (!sessionId || turnIds.length === 0) return;
      const wanted = new Set(turnIds);
      while (true) {
        const state = store.getState().turnChanges;
        const available = new Set(
          selectTurnChangeSummaries(state, sessionId).map((summary) => summary.turn_id),
        );
        if ([...wanted].every((turnId) => available.has(turnId))) return;
        const offset = state.nextOffsetBySession[sessionId];
        if (offset == null) return;
        const page = await loadPage(sessionId, offset, store);
        if (page.next_offset == null) return;
      }
    },
    [sessionId, store],
  );

  const loadChangeSet = useCallback(
    async (changeSetId: string) => {
      if (!sessionId || !changeSetId) return;
      const current = selectTurnChangeSummaries(store.getState().turnChanges, sessionId);
      if (current.some((summary) => summary.id === changeSetId)) return;
      setLoading(true);
      setError(null);
      try {
        const summary = await getTurnChangeHistoryItem(sessionId, changeSetId, {
          cache: "no-store",
        });
        if (store.getState().taskSessions.items[sessionId]) {
          store.getState().mergeTurnChangeSummary(sessionId, summary);
        }
      } catch (err) {
        setError(err);
      } finally {
        setLoading(false);
      }
    },
    [sessionId, store],
  );

  return {
    summaries,
    loading,
    error,
    hasMore: nextOffset !== null,
    loadMore,
    ensureTurnsLoaded,
    loadChangeSet,
    reload: () => load(0),
  };
}
