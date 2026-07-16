"use client";

import { useEffect, useState } from "react";
import { discoverRepositoriesAction } from "@/app/actions/workspaces";
import type { LocalRepository } from "@/lib/types/http";

type DiscoveryResult = { ws: string; repos: LocalRepository[] };

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
  const [result, setResult] = useState<DiscoveryResult | null>(null);

  useEffect(() => {
    if (!open || !workspaceId) return;
    if (result?.ws === workspaceId) return;
    let cancelled = false;
    discoverRepositoriesAction(workspaceId)
      .then((res) => {
        if (!cancelled) setResult({ ws: workspaceId, repos: res.repositories ?? [] });
      })
      .catch(() => {
        if (!cancelled) setResult({ ws: workspaceId, repos: [] });
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, result]);

  return result?.ws === workspaceId ? result.repos : null;
}
