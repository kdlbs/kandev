"use client";

import { useEffect, useCallback } from "react";
import { listPRWatches, deletePRWatch } from "@/lib/api/domains/github-api";
import { useAppStore } from "@/components/state-provider";

export function usePRWatches() {
  const items = useAppStore((state) => state.prWatches.items);
  const loaded = useAppStore((state) => state.prWatches.loaded);
  const loading = useAppStore((state) => state.prWatches.loading);
  const setPRWatches = useAppStore((state) => state.setPRWatches);
  const setPRWatchesLoading = useAppStore((state) => state.setPRWatchesLoading);
  const removePRWatch = useAppStore((state) => state.removePRWatch);

  useEffect(() => {
    if (loaded || loading) return;
    setPRWatchesLoading(true);
    listPRWatches({ cache: "no-store" })
      .then((response) => {
        setPRWatches(response?.watches ?? []);
      })
      .catch(() => {
        setPRWatches([]);
      })
      .finally(() => {
        setPRWatchesLoading(false);
      });
  }, [loaded, loading, setPRWatches, setPRWatchesLoading]);

  const remove = useCallback(
    async (id: string) => {
      await deletePRWatch(id);
      removePRWatch(id);
    },
    [removePRWatch],
  );

  return { items, loaded, loading, remove };
}

/** Get the PR watch for a specific session. */
export function usePRWatchForSession(sessionId: string | null) {
  const items = useAppStore((state) => state.prWatches.items);
  const watch = sessionId ? (items.find((w) => w.session_id === sessionId) ?? null) : null;
  return watch;
}
