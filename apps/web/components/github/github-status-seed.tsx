"use client";

import { useLayoutEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { qk } from "@/lib/query/keys";
import type { GitHubStatusResponse } from "@/lib/types/github";

/**
 * Seeds the GitHub status TQ cache from an SSR snapshot. This page hydrates
 * Zustand (no TQ HydrationBoundary), so without seeding, `useGitHubStatus`
 * mounts with an empty cache and flashes a "loading" state until the client
 * refetch lands. Seed-if-absent so a live result is never clobbered.
 */
export function GitHubStatusSeed({ status }: { status: GitHubStatusResponse | null }) {
  const queryClient = useQueryClient();
  useLayoutEffect(() => {
    if (status && !queryClient.getQueryData(qk.github.status())) {
      queryClient.setQueryData(qk.github.status(), status);
    }
  }, [status, queryClient]);
  return null;
}
