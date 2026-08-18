import { useCallback, useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import type { RepositorySet } from "@/lib/types/http";
import { listRepositorySets } from "@/lib/api";

const EMPTY_REPOSITORY_SETS: RepositorySet[] = [];

/**
 * Loads a workspace's repository sets from the store, fetching once when not yet
 * loaded.
 *
 * The boot payload hydrates this slice for every route that can open the task
 * dialog, so the common case does no fetch at all. A workspace with no sets is
 * still marked loaded at boot, which is why the fetch is gated on `isLoaded`
 * rather than on the list being non-empty.
 */
export function useRepositorySets(workspaceId: string | null, enabled = true) {
  const sets = useAppStore((state) =>
    workspaceId
      ? (state.repositorySets.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORY_SETS)
      : EMPTY_REPOSITORY_SETS,
  );
  const isLoading = useAppStore((state) =>
    workspaceId ? (state.repositorySets.loadingByWorkspaceId[workspaceId] ?? false) : false,
  );
  const isLoaded = useAppStore((state) =>
    workspaceId ? (state.repositorySets.loadedByWorkspaceId[workspaceId] ?? false) : false,
  );
  const setRepositorySets = useAppStore((state) => state.setRepositorySets);
  const setRepositorySetsLoading = useAppStore((state) => state.setRepositorySetsLoading);
  // Selecting the number (not the whole store) keeps this a re-render only when
  // the revision actually moves; the ref lets an in-flight request read the
  // value as of its own start rather than a stale closure capture.
  const revision = useAppStore((state) =>
    workspaceId ? (state.repositorySets.revisionByWorkspaceId[workspaceId] ?? 0) : 0,
  );
  const revisionRef = useRef(revision);
  revisionRef.current = revision;

  const refresh = useCallback(async () => {
    if (!enabled || !workspaceId) return;
    setRepositorySetsLoading(workspaceId, true);
    const requestRevision = revisionRef.current;
    try {
      const response = await listRepositorySets(workspaceId, { cache: "no-store" });
      setRepositorySets(workspaceId, response.repository_sets, requestRevision);
    } catch {
      // Keep the cached sets when a manual refresh fails.
    } finally {
      setRepositorySetsLoading(workspaceId, false);
    }
  }, [enabled, setRepositorySets, setRepositorySetsLoading, workspaceId]);

  useEffect(() => {
    if (!enabled || !workspaceId || isLoaded) return;
    let cancelled = false;
    setRepositorySetsLoading(workspaceId, true);
    const requestRevision = revisionRef.current;
    listRepositorySets(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        setRepositorySets(workspaceId, response.repository_sets, requestRevision);
      })
      .catch(() => {
        // Leave the workspace unloaded after a failure so the next mount can
        // retry, rather than treating an empty fallback as a real answer.
      })
      .finally(() => {
        if (cancelled) return;
        setRepositorySetsLoading(workspaceId, false);
      });
    return () => {
      cancelled = true;
    };
  }, [enabled, isLoaded, workspaceId, setRepositorySets, setRepositorySetsLoading]);

  return { sets, isLoading, refresh };
}
