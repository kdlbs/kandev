"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchRepoBranches } from "@/lib/api/domains/github-api";
import { parseGitHubRepoUrl } from "@/lib/github/parse-url";
import type { Branch } from "@/lib/types/http";

/**
 * Per-URL branches loader for GitHub remote-repo URLs. Lifted from the single-
 * URL branches loader inside `task-create-dialog-state.ts` so the new GitHub
 * Remote tab (Task 4) can drive branch loading for several URLs at once
 * without rebuilding the per-URL effect at every chip.
 *
 * Behavior:
 *   - `ensure(url)` triggers a fetch the first time a URL is seen; subsequent
 *     `ensure(url)` calls (concurrent or sequential) are no-ops while a fetch
 *     is in flight or after one has settled.
 *   - `ensure("")` (empty string) is a no-op — useful when the chip's URL has
 *     been cleared and the caller still wants to call `ensure` unconditionally.
 *   - `branches(url)` returns the most-recently loaded branch list for `url`,
 *     or `[]` if none has been loaded.
 *   - `loading(url)` returns true while a fetch for `url` is in flight.
 *
 * Per-URL state is scoped to the hook instance, not the module — two callers
 * of this hook won't share cache. That mirrors how the dialog uses it today
 * (single owning component) and avoids leaking branch lists across unrelated
 * dialogs.
 */

type URLState = {
  branches: Branch[];
  loading: boolean;
};

export type UseBranchesByURLResult = {
  branches: (url: string) => Branch[];
  loading: (url: string) => boolean;
  ensure: (url: string) => void;
  /**
   * Forget the cached entry for `url` so the next `ensure(url)` re-fetches.
   * Aborts any in-flight request and discards any pending callbacks via the
   * per-URL sequence counter. Use after a failed fetch to retry.
   */
  clear: (url: string) => void;
};

const EMPTY: Branch[] = [];

export function useBranchesByURL(): UseBranchesByURLResult {
  const [state, setState] = useState<Record<string, URLState>>({});
  // Tracks in-flight URLs so concurrent ensure() calls coalesce. We use a ref
  // (not state) because the dedup check must observe the latest value
  // synchronously across ensure() calls in the same tick.
  const inFlightRef = useRef<Set<string>>(new Set());
  const loadedRef = useRef<Set<string>>(new Set());
  const abortersRef = useRef<Map<string, AbortController>>(new Map());
  // Per-URL request sequence. Incremented on every fetch and on clear();
  // settled callbacks compare against the latest value and bail when a newer
  // request has superseded them. Prevents a stale fetch from clobbering the
  // state set by a later ensure() for the same URL.
  const requestSeqByURLRef = useRef<Map<string, number>>(new Map());
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    // Snapshot the refs into local consts so the cleanup function reads from
    // the same object identity the effect captured at mount, even if the
    // (frozen) ref instance changes — silences exhaustive-deps' ref-cleanup
    // warning without changing behavior, since these refs are never reassigned.
    const aborters = abortersRef.current;
    const inFlight = inFlightRef.current;
    const loaded = loadedRef.current;
    const seqs = requestSeqByURLRef.current;
    return () => {
      mountedRef.current = false;
      for (const controller of aborters.values()) controller.abort();
      aborters.clear();
      inFlight.clear();
      loaded.clear();
      seqs.clear();
    };
  }, []);

  const ensure = useCallback((url: string) => {
    if (!url) return;
    if (inFlightRef.current.has(url) || loadedRef.current.has(url)) return;
    const parsed = parseGitHubRepoUrl(url);
    if (!parsed) {
      loadedRef.current.add(url);
      setState((prev) => ({
        ...prev,
        [url]: { branches: [], loading: false },
      }));
      return;
    }
    setState((prev) => ({
      ...prev,
      [url]: {
        branches: prev[url]?.branches ?? [],
        loading: true,
      },
    }));
    inFlightRef.current.add(url);
    const controller = new AbortController();
    abortersRef.current.set(url, controller);
    const seq = (requestSeqByURLRef.current.get(url) ?? 0) + 1;
    requestSeqByURLRef.current.set(url, seq);

    fetchRepoBranches(parsed.owner, parsed.repo, {
      init: { signal: controller.signal },
    })
      .then((res) => {
        if (!mountedRef.current) return;
        if (requestSeqByURLRef.current.get(url) !== seq) return;
        const branches: Branch[] = (res?.branches ?? []).map((b) => ({
          name: b.name,
          type: "remote" as const,
        }));
        loadedRef.current.add(url);
        setState((prev) => ({
          ...prev,
          [url]: { branches, loading: false },
        }));
      })
      .catch(() => {
        if (!mountedRef.current) return;
        if (requestSeqByURLRef.current.get(url) !== seq) return;
        // Don't mark `loaded` on failure — leaving it unmarked allows the
        // next ensure() call for the same URL to retry instead of
        // short-circuiting on the cached failure. Callers can also call
        // clear(url) to force a refetch.
        setState((prev) => ({
          ...prev,
          [url]: {
            branches: prev[url]?.branches ?? [],
            loading: false,
          },
        }));
      })
      .finally(() => {
        if (requestSeqByURLRef.current.get(url) !== seq) return;
        inFlightRef.current.delete(url);
        abortersRef.current.delete(url);
      });
  }, []);

  const clear = useCallback((url: string) => {
    if (!url) return;
    inFlightRef.current.delete(url);
    loadedRef.current.delete(url);
    requestSeqByURLRef.current.set(url, (requestSeqByURLRef.current.get(url) ?? 0) + 1);
    const aborter = abortersRef.current.get(url);
    if (aborter) {
      aborter.abort();
      abortersRef.current.delete(url);
    }
    setState((prev) => {
      if (!(url in prev)) return prev;
      const next = { ...prev };
      delete next[url];
      return next;
    });
  }, []);

  const branches = useCallback((url: string): Branch[] => state[url]?.branches ?? EMPTY, [state]);
  const loading = useCallback((url: string): boolean => Boolean(state[url]?.loading), [state]);

  return { branches, loading, ensure, clear };
}
