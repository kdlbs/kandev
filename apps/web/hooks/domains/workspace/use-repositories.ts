import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";
import { listRepositories } from "@/lib/api";

const EMPTY_REPOSITORIES: Repository[] = [];

/**
 * Loads a workspace's repositories from the store, fetching once when not yet
 * loaded. Pass `forceRefresh` to instead pull a fresh list once per workspace on
 * mount (e.g. a picker that must reflect repos created since the slice was first
 * loaded) — the lazy path is disabled in that mode so there's no double fetch,
 * and the per-workspace guard is only marked on success so a failed fetch can
 * retry on the next mount.
 */
export function useRepositories(workspaceId: string | null, enabled = true, forceRefresh = false) {
  const repositories = useAppStore((state) =>
    workspaceId
      ? (state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES)
      : EMPTY_REPOSITORIES,
  );
  const isLoading = useAppStore((state) =>
    workspaceId ? (state.repositories.loadingByWorkspaceId[workspaceId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    workspaceId ? (state.repositories.loadedByWorkspaceId[workspaceId] ?? false) : false,
  );
  const setRepositories = useAppStore((state) => state.setRepositories);
  const setRepositoriesLoading = useAppStore((state) => state.setRepositoriesLoading);
  // Tracks the workspace whose fetch is currently in flight (not a bare bool):
  // a fetch for one workspace must not block a fetch for another when the
  // selected workspace changes mid-request.
  const inFlightRef = useRef<string | null>(null);
  const forcedRef = useRef<string | null>(null);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    if (isLoaded && isLoading) {
      setRepositoriesLoading(workspaceId, false);
    }
  }, [enabled, isLoaded, isLoading, setRepositoriesLoading, workspaceId]);

  // Force-refresh: pull a fresh list once per workspace, bypassing the
  // isLoaded cache. forcedRef is set only on success so a failed fetch retries.
  useEffect(() => {
    if (!enabled || !workspaceId || !forceRefresh) return;
    if (forcedRef.current === workspaceId || inFlightRef.current === workspaceId) return;
    let cancelled = false;
    inFlightRef.current = workspaceId;
    setRepositoriesLoading(workspaceId, true);
    listRepositories(workspaceId, undefined, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        forcedRef.current = workspaceId;
        setRepositories(workspaceId, response.repositories);
      })
      .catch(() => {
        // Leave forcedRef unset so the next mount retries; keep cached repos.
      })
      .finally(() => {
        if (inFlightRef.current === workspaceId) inFlightRef.current = null;
        if (cancelled) return;
        setRepositoriesLoading(workspaceId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, forceRefresh, workspaceId, setRepositories, setRepositoriesLoading]);

  useEffect(() => {
    if (!enabled || !workspaceId || forceRefresh) return;
    if (isLoaded || inFlightRef.current === workspaceId) return;
    let cancelled = false;
    inFlightRef.current = workspaceId;
    setRepositoriesLoading(workspaceId, true);
    listRepositories(workspaceId, undefined, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        setRepositories(workspaceId, response.repositories);
      })
      .catch(() => {
        if (cancelled) return;
        setRepositories(workspaceId, []);
      })
      .finally(() => {
        if (inFlightRef.current === workspaceId) inFlightRef.current = null;
        if (cancelled) return;
        setRepositoriesLoading(workspaceId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, forceRefresh, isLoaded, setRepositories, setRepositoriesLoading, workspaceId]);

  return { repositories, isLoading };
}
