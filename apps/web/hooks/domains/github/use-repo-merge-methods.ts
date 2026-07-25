"use client";

import { useEffect, useReducer } from "react";
import { getRepoMergeMethods } from "@/lib/api/domains/github-api";
import type { RepoMergeMethods } from "@/lib/types/github";

// Module-level cache: merge settings are repo-scoped and rarely change, and
// the backend already TTL-caches them. Coalescing concurrent fetches here
// keeps multiple merge buttons on the same page from triggering duplicate
// round-trips before the backend cache warms. Match the backend's 5-minute
// window so an admin flipping a merge strategy mid-session is reflected
// without a full page reload.
const CACHE_TTL_MS = 5 * 60 * 1000;

type CachedEntry = { value: RepoMergeMethods; expiresAt: number };
const completed = new Map<string, CachedEntry>();
// The pending promise is only used to coalesce concurrent fetches — consumers
// re-read from `completed` after it settles, so the resolved value is irrelevant.
// Typing it as `void` lets the chain swallow rejections cleanly (no rethrow,
// no unhandled-rejection warnings in the browser console).
const pending = new Map<string, Promise<void>>();

function repoKey(workspaceId: string, owner: string, repo: string) {
  return `${workspaceId}:${owner}/${repo}`;
}

function readFresh(key: string): RepoMergeMethods | null {
  const entry = completed.get(key);
  if (!entry) return null;
  if (entry.expiresAt <= Date.now()) {
    completed.delete(key);
    return null;
  }
  return entry.value;
}

/**
 * Fetches the merge methods a repository allows so callers can populate the
 * merge-method dropdown without sending a request to GitHub that 405s on
 * squash-only / rebase-only repos.
 *
 * Returns `null` while the fetch is in flight, on failure, or when
 * owner/repo are missing. Callers should treat `null` as "use the backend
 * resolver" — never as "hide the merge UI", because a transient lookup
 * failure would otherwise lock the user out of merging.
 */
export function useRepoMergeMethods(
  workspaceId: string | null,
  owner: string | null,
  repo: string | null,
): RepoMergeMethods | null {
  const [, rerender] = useReducer((c: number) => c + 1, 0);

  useEffect(() => {
    if (!workspaceId || !owner || !repo) return;
    const key = repoKey(workspaceId, owner, repo);
    if (readFresh(key)) return;
    const existing = pending.get(key);
    if (existing) {
      void existing.then(rerender);
      return;
    }
    const promise = getRepoMergeMethods(workspaceId, owner, repo)
      .then((result) => {
        completed.set(key, { value: result, expiresAt: Date.now() + CACHE_TTL_MS });
        pending.delete(key);
        rerender();
      })
      .catch(() => {
        // Don't poison the cache on transient failures — the next mount or
        // render will retry. Swallow the rejection here so the originating
        // consumer's promise doesn't surface as an unhandled rejection in
        // the browser console; co-waiting consumers re-render and read
        // null, which behaves identically to a fresh "not-loaded" state.
        pending.delete(key);
        rerender();
      });
    pending.set(key, promise);
  }, [workspaceId, owner, repo]);

  if (!workspaceId || !owner || !repo) return null;
  return readFresh(repoKey(workspaceId, owner, repo));
}
