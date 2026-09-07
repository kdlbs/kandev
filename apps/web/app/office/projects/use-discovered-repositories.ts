"use client";

import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";
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
  const discovery = useRepositoryDiscovery(workspaceId, open);
  if (!open || !workspaceId || !discovery.hasSnapshot) return null;
  return discovery.repositories;
}
