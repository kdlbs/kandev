"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { deletePRWatch } from "@/lib/api/domains/github-api";
import { qk } from "@/lib/query/keys";
import { prWatchesQueryOptions } from "@/lib/query/query-options/github";

export function usePRWatches() {
  const queryClient = useQueryClient();
  const query = useQuery(prWatchesQueryOptions());
  const items = query.data ?? [];

  const remove = useCallback(
    async (id: string) => {
      await deletePRWatch(id);
      queryClient.setQueryData(qk.integrations.github.prWatches(), (prev: typeof items) =>
        (prev ?? []).filter((watch) => watch.id !== id),
      );
    },
    [items, queryClient],
  );

  return {
    items,
    loaded: query.isSuccess,
    loading: query.isFetching && !query.isSuccess,
    remove,
  };
}

/** Get the PR watch for a specific session. */
export function usePRWatchForSession(sessionId: string | null) {
  const query = useQuery(prWatchesQueryOptions());
  const items = query.data ?? [];
  const watch = sessionId ? (items.find((w) => w.session_id === sessionId) ?? null) : null;
  return watch;
}
