"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { listPRWatches, deletePRWatch } from "@/lib/api/domains/github-api";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import { qk } from "@/lib/query/keys";
import type { PRWatch } from "@/lib/types/github";

export function usePRWatches() {
  const qc = useQueryClient();
  const { data, isLoading, isSuccess } = useQuery(githubQueryOptions.prWatches());

  const removeMutation = useMutation({
    mutationFn: (id: string) => deletePRWatch(id),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: qk.github.prWatches() });
      const prev = qc.getQueryData<{ watches: PRWatch[] }>(qk.github.prWatches());
      qc.setQueryData<{ watches: PRWatch[] }>(qk.github.prWatches(), (old) => {
        if (!old) return old;
        return { ...old, watches: old.watches.filter((w) => w.id !== id) };
      });
      return { prev };
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev !== undefined) {
        qc.setQueryData(qk.github.prWatches(), ctx.prev);
      }
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.github.prWatches() });
    },
  });

  return {
    items: data?.watches ?? [],
    loaded: isSuccess,
    loading: isLoading,
    remove: (id: string) => removeMutation.mutateAsync(id),
  };
}

/** Get the PR watch for a specific session. */
export function usePRWatchForSession(sessionId: string | null): PRWatch | null {
  const { data } = useQuery({
    ...githubQueryOptions.prWatches(),
    select: (d) => (sessionId ? (d.watches.find((w) => w.session_id === sessionId) ?? null) : null),
  });
  return data ?? null;
}

// Re-export for backwards compat
export { listPRWatches };
