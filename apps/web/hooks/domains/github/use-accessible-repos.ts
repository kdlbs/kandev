"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchAccessibleRepos,
  GitHubUnavailableError,
  type AccessibleRepo,
} from "@/lib/api/domains/github-api";

const DEBOUNCE_MS = 250;
const DEFAULT_LIMIT = 50;

export type UseAccessibleReposResult = {
  repos: AccessibleRepo[];
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
 *   - Fetches accessible repos for the initial query on mount (default "").
 *   - `search(q)` resets a 250ms debounce; the last query inside the window
 *     is the only one that fires.
 *   - Per-query memoization: a query that has already loaded resolves
 *     synchronously from cache; no extra fetch.
 *   - Switching queries aborts the in-flight request for the previous one.
 *   - 503 with `code: github_not_configured` surfaces as `unavailable: true`,
 *     not as a generic `error` — the UI shows a "Connect GitHub" CTA.
 *   - Other errors surface via the `error` field.
 *   - Unmount aborts any pending fetch.
 */
export function useAccessibleRepos(initialQuery: string = ""): UseAccessibleReposResult {
  const [query, setQuery] = useState(initialQuery);
  const [repos, setRepos] = useState<AccessibleRepo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  const cacheRef = useRef<Map<string, AccessibleRepo[]>>(new Map());
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (debounceRef.current) clearTimeout(debounceRef.current);
      abortRef.current?.abort();
    };
  }, []);

  // Drives the actual fetch when `query` changes. Cache hits short-circuit;
  // otherwise the debounce timer fires off a fresh request and aborts any
  // previous in-flight one.
  useEffect(() => {
    // Whitespace-only queries collapse to "" so they share the empty-query
    // cache entry and don't issue a fetch with `q=%20%20%20`.
    const normalized = query.trim();
    const cached = cacheRef.current.get(normalized);
    if (cached) {
      setRepos(cached);
      setLoading(false);
      setError(null);
      setUnavailable(false);
      // Cancel any in-flight from a previous query — its result would
      // otherwise overwrite the cache hit.
      abortRef.current?.abort();
      abortRef.current = null;
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
        debounceRef.current = null;
      }
      return;
    }
    if (debounceRef.current) clearTimeout(debounceRef.current);
    setLoading(true);
    setError(null);
    setUnavailable(false);
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    debounceRef.current = setTimeout(() => {
      runFetch(normalized, controller);
    }, DEBOUNCE_MS);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };

    function runFetch(q: string, ctl: AbortController) {
      fetchAccessibleRepos({ q: q || undefined, limit: DEFAULT_LIMIT, signal: ctl.signal })
        .then((result) => {
          if (!mountedRef.current || ctl.signal.aborted) return;
          cacheRef.current.set(q, result);
          setRepos(result);
          setLoading(false);
          setError(null);
          setUnavailable(false);
        })
        .catch((err: unknown) => {
          if (!mountedRef.current || ctl.signal.aborted) return;
          if (err instanceof GitHubUnavailableError) {
            setUnavailable(true);
            setError(null);
            setRepos([]);
            setLoading(false);
            return;
          }
          if (err instanceof DOMException && err.name === "AbortError") return;
          // Clear stale `repos` so the UI doesn't render the previous
          // query's results next to the new query's error banner. We don't
          // touch cacheRef — the previous query's entry stays cached for
          // when the user navigates back to it.
          setError(err instanceof Error ? err : new Error(String(err)));
          setRepos([]);
          setLoading(false);
        });
    }
  }, [query]);

  const search = useCallback((q: string) => {
    setQuery(q);
  }, []);

  return { repos, loading, error, unavailable, search };
}
