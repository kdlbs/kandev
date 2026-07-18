"use client";

import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { discoveredRepositoriesQueryOptions } from "@/lib/query/query-options";
import type { LocalRepository } from "@/lib/types/http";

/**
 * Lazily discovers on-disk repositories while the picker popover is
 * open. Returns `null` until the current workspace's discovery has
 * resolved — used to drive the "Searching your machine…" empty-state
 * copy without an extra loading flag.
 *
 * The result is keyed by workspace id and derived on read, so a
 * workspace switch immediately yields `null` (never another
 * workspace's paths) and triggers a fresh scan, and a request
 * interrupted by closing the popover simply retries on reopen
 * instead of latching a never-resolved state.
 */
export function useDiscoveredRepositories(
  open: boolean,
  workspaceId: string | null,
): LocalRepository[] | null {
  const queryClient = useQueryClient();
  const query = useQuery({
    ...discoveredRepositoriesQueryOptions(workspaceId ?? ""),
    enabled: open && Boolean(workspaceId),
  });
  useEffect(() => {
    if (open || !workspaceId) return;
    void queryClient.cancelQueries({
      exact: true,
      queryKey: discoveredRepositoriesQueryOptions(workspaceId).queryKey,
    });
  }, [open, queryClient, workspaceId]);
  if (!open || !workspaceId || !query.isFetched) return null;
  return query.data ?? [];
}
