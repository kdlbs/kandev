"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createReviewWatch,
  updateReviewWatch,
  deleteReviewWatch,
  triggerReviewWatch,
  triggerAllReviewWatches,
} from "@/lib/api/domains/github-api";
import { githubQueryOptions } from "@/lib/query/query-options/github";
import { qk } from "@/lib/query/keys";
import type {
  ReviewWatch,
  CreateReviewWatchRequest,
  UpdateReviewWatchRequest,
} from "@/lib/types/github";

// useReviewWatches has three modes:
//   - workspaceId: string         → fetch watches scoped to one workspace
//   - workspaceId: undefined      → fetch watches across all workspaces
//   - workspaceId: null           → don't fetch (caller hasn't resolved a workspace yet)
export function useReviewWatches(workspaceId?: string | null) {
  const qc = useQueryClient();
  const cacheKey =
    workspaceId !== null
      ? qk.github.reviewWatches(workspaceId ?? undefined)
      : qk.github.reviewWatches();

  const { data, isLoading, isSuccess } = useQuery(githubQueryOptions.reviewWatches(workspaceId));

  const createMutation = useMutation({
    mutationFn: (req: CreateReviewWatchRequest) => createReviewWatch(req),
    onSuccess: (watch) => {
      qc.setQueryData<{ watches: ReviewWatch[] }>(cacheKey, (prev) => {
        if (!prev) return { watches: [watch] };
        return {
          ...prev,
          watches: [...prev.watches.filter((w) => w.id !== watch.id), watch],
        };
      });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: cacheKey });
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, req }: { id: string; req: UpdateReviewWatchRequest }) =>
      updateReviewWatch(id, req),
    onSuccess: (watch) => {
      qc.setQueryData<{ watches: ReviewWatch[] }>(cacheKey, (prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          watches: prev.watches.map((w) => (w.id === watch.id ? watch : w)),
        };
      });
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: cacheKey });
    },
  });

  const removeMutation = useMutation({
    mutationFn: (id: string) => deleteReviewWatch(id),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: cacheKey });
      const prev = qc.getQueryData<{ watches: ReviewWatch[] }>(cacheKey);
      qc.setQueryData<{ watches: ReviewWatch[] }>(cacheKey, (old) => {
        if (!old) return old;
        return { ...old, watches: old.watches.filter((w) => w.id !== id) };
      });
      return { prev };
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev !== undefined) qc.setQueryData(cacheKey, ctx.prev);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: cacheKey });
    },
  });

  async function create(req: CreateReviewWatchRequest) {
    return createMutation.mutateAsync(req);
  }

  async function update(id: string, req: UpdateReviewWatchRequest) {
    return updateMutation.mutateAsync({ id, req });
  }

  async function remove(id: string) {
    return removeMutation.mutateAsync(id);
  }

  async function trigger(id: string) {
    return triggerReviewWatch(id);
  }

  async function triggerAll() {
    if (!workspaceId) return null;
    return triggerAllReviewWatches(workspaceId);
  }

  return {
    items: data?.watches ?? [],
    loaded: isSuccess,
    loading: isLoading,
    create,
    update,
    remove,
    trigger,
    triggerAll,
  };
}
