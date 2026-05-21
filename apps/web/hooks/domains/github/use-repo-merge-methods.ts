"use client";

import { useEffect, useReducer } from "react";
import { getRepoMergeMethods } from "@/lib/api/domains/github-api";
import type { RepoMergeMethods } from "@/lib/types/github";

// Module-level cache: merge settings are repo-scoped and rarely change, and
// the backend already TTL-caches them. Coalescing concurrent fetches here
// keeps multiple merge buttons on the same page from triggering duplicate
// round-trips before the backend cache warms.
const completed = new Map<string, RepoMergeMethods>();
const pending = new Map<string, Promise<RepoMergeMethods>>();

function repoKey(owner: string, repo: string) {
  return `${owner}/${repo}`;
}

/**
 * Fetches the merge methods a repository allows so callers can hide
 * disallowed options and avoid the 405 GitHub returns when an empty
 * merge_method falls through to the default "merge" on squash-only repos.
 *
 * Returns `null` while the fetch is in flight (or owner/repo are missing)
 * so consumers can defer rendering until the answer is known.
 */
export function useRepoMergeMethods(
  owner: string | null,
  repo: string | null,
): RepoMergeMethods | null {
  const [, rerender] = useReducer((c: number) => c + 1, 0);

  useEffect(() => {
    if (!owner || !repo) return;
    const key = repoKey(owner, repo);
    if (completed.has(key)) return;
    const existing = pending.get(key);
    if (existing) {
      void existing.then(rerender);
      return;
    }
    const promise = getRepoMergeMethods(owner, repo)
      .then((result) => {
        completed.set(key, result);
        pending.delete(key);
        rerender();
        return result;
      })
      .catch((err) => {
        // Don't poison the cache on transient failures — let the next mount retry.
        pending.delete(key);
        throw err;
      });
    pending.set(key, promise);
  }, [owner, repo]);

  if (!owner || !repo) return null;
  return completed.get(repoKey(owner, repo)) ?? null;
}
