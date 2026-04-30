import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listRepositoryBranches } from "@/lib/api";
import type { Branch } from "@/lib/types/http";

const EMPTY_BRANCHES: Branch[] = [];

// Stale-while-revalidate: when the cache is older than this we fire a
// background refresh on hook mount/repo change. The backend additionally
// enforces a per-repo cooldown so this is the soft, client-side check.
const CLIENT_STALE_MS = 60_000;

export function useRepositoryBranches(repositoryId: string | null, enabled = true) {
  const branches = useAppStore((state) =>
    repositoryId
      ? (state.repositoryBranches.itemsByRepositoryId[repositoryId] ?? EMPTY_BRANCHES)
      : EMPTY_BRANCHES,
  );
  const isLoaded = useAppStore((state) =>
    repositoryId ? (state.repositoryBranches.loadedByRepositoryId[repositoryId] ?? false) : false,
  );
  const isLoading = useAppStore((state) =>
    repositoryId ? (state.repositoryBranches.loadingByRepositoryId[repositoryId] ?? false) : false,
  );
  const fetchedAt = useAppStore((state) =>
    repositoryId ? state.repositoryBranches.fetchedAtByRepositoryId[repositoryId] : undefined,
  );
  const fetchError = useAppStore((state) =>
    repositoryId ? state.repositoryBranches.fetchErrorByRepositoryId[repositoryId] : undefined,
  );
  const setRepositoryBranches = useAppStore((state) => state.setRepositoryBranches);
  const setRepositoryBranchesLoading = useAppStore((state) => state.setRepositoryBranchesLoading);
  const inFlightRef = useRef<string | null>(null);

  // Stable fetcher closed over the store actions; the repo id is a parameter
  // so the same identity works for both mount-time fetches and refresh().
  const runFetch = useCallback(
    (repoId: string, refresh: boolean) => {
      if (inFlightRef.current === repoId) return;
      inFlightRef.current = repoId;
      setRepositoryBranchesLoading(repoId, true);
      listRepositoryBranches(repoId, { refresh }, { cache: "no-store" })
        .then((response) => {
          setRepositoryBranches(repoId, response.branches, {
            fetchedAt: response.fetched_at,
            fetchError: response.fetch_error,
          });
        })
        .catch(() => {
          setRepositoryBranches(repoId, []);
        })
        .finally(() => {
          if (inFlightRef.current === repoId) inFlightRef.current = null;
          setRepositoryBranchesLoading(repoId, false);
        });
    },
    [setRepositoryBranches, setRepositoryBranchesLoading],
  );

  useEffect(() => {
    if (!enabled || !repositoryId) return;
    // Cold load: nothing in the cache, do a foreground fetch with refresh so
    // the user sees fresh remote branches the first time the dialog opens.
    if (!isLoaded) {
      runFetch(repositoryId, true);
      return;
    }
    // Warm cache, possibly stale: revalidate in the background. The backend
    // cooldown ensures this doesn't hammer git when the dialog is reopened
    // in quick succession.
    const ageMs = fetchedAt ? Date.now() - new Date(fetchedAt).getTime() : Infinity;
    if (ageMs >= CLIENT_STALE_MS) {
      runFetch(repositoryId, true);
    }
  }, [enabled, isLoaded, repositoryId, fetchedAt, runFetch]);

  const refresh = useCallback(() => {
    if (!repositoryId) return;
    runFetch(repositoryId, true);
  }, [repositoryId, runFetch]);

  return { branches, isLoading, fetchedAt, fetchError, refresh };
}
