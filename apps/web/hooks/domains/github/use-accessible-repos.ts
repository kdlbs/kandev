"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  fetchAccessibleRepos,
  GitHubUnavailableError,
  type AccessibleRepo,
} from "@/lib/api/domains/github-api";

// Cap matches the backend's maxAccessibleReposLimit. The backend caps per_page
// at 100, so requesting more wastes effort. Users with >100 accessible repos
// need to paste a URL for repos that fall off the list.
const INITIAL_LIMIT = 100;

export type UseAccessibleReposResult = {
  /** Filtered subset of the full list — repos whose full_name matches the current search (case-insensitive substring). */
  repos: AccessibleRepo[];
  /** True while the initial mount-time fetch is in flight. False thereafter. */
  loading: boolean;
  error: Error | null;
  unavailable: boolean;
  search: (q: string) => void;
};

/**
 * Drives the autocomplete picker for the GitHub Remote tab in the task-
 * create dialog.
 *
 * Behavior:
 *   - Fetches the full accessible-repo list once on mount with limit=100
 *     (the backend's cap). Stores the full list in a ref.
 *   - `search(q)` filters the cached list client-side as a case-insensitive
 *     substring match against `full_name` — NO additional backend requests.
 *   - `loading` is only true during the initial fetch.
 *   - 503 with `code: github_not_configured` surfaces as `unavailable: true`,
 *     not as a generic `error` — the UI shows a "Connect GitHub" CTA.
 *   - Other errors surface via the `error` field.
 *   - Unmount aborts the initial fetch if still in flight.
 *
 * Rationale: the previous implementation fired a backend request on every
 * keystroke (debounced 250ms). The backend fans out N+1 GitHub API calls per
 * request (1 user-repos + N org searches), which trips gh-cli rate limits for
 * users with several orgs. The 60s backend cache helped within a single
 * picker session but couldn't save the user during typeahead bursts. Moving
 * the filter client-side trades a one-shot full-list fetch (already cached
 * server-side) for zero per-keystroke load.
 */
export function useAccessibleRepos(
  workspaceId: string | null,
  initialQuery: string = "",
): UseAccessibleReposResult {
  const [allRepos, setAllRepos] = useState<AccessibleRepo[]>([]);
  const [query, setQuery] = useState(initialQuery);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!workspaceId) {
      setAllRepos([]);
      setLoading(false);
      setError(null);
      setUnavailable(false);
      return;
    }
    setLoading(true);
    const controller = new AbortController();
    abortRef.current = controller;
    let cancelled = false;

    fetchAccessibleRepos({ workspaceId, limit: INITIAL_LIMIT, signal: controller.signal })
      .then((result) => {
        if (cancelled || controller.signal.aborted) return;
        setAllRepos(result);
        setLoading(false);
        setError(null);
        setUnavailable(false);
      })
      .catch((err: unknown) => {
        if (cancelled || controller.signal.aborted) return;
        if (err instanceof GitHubUnavailableError) {
          setUnavailable(true);
          setError(null);
          setAllRepos([]);
          setLoading(false);
          return;
        }
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err : new Error(String(err)));
        setAllRepos([]);
        setLoading(false);
      });

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [workspaceId]);

  const repos = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return allRepos;
    return allRepos.filter((r) => r.full_name.toLowerCase().includes(normalized));
  }, [allRepos, query]);

  const search = useCallback((q: string) => {
    setQuery(q);
  }, []);

  return { repos, loading, error, unavailable, search };
}
