"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchRepoBranches } from "@/lib/api/domains/github-api";
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
  loaded: boolean;
};

export type UseBranchesByURLResult = {
  branches: (url: string) => Branch[];
  loading: (url: string) => boolean;
  ensure: (url: string) => void;
};

const EMPTY: Branch[] = [];

function parseGitHubRepoURL(url: string): { owner: string; repo: string } | null {
  const trimmed = url.trim();
  if (!trimmed) return null;
  const match = trimmed.match(
    /(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+?)(?:\.git)?\/?$/,
  );
  if (!match) return null;
  return { owner: match[1], repo: match[2] };
}

export function useBranchesByURL(): UseBranchesByURLResult {
  const [state, setState] = useState<Record<string, URLState>>({});
  // Tracks in-flight URLs so concurrent ensure() calls coalesce. We use a ref
  // (not state) because the dedup check must observe the latest value
  // synchronously across ensure() calls in the same tick.
  const inFlightRef = useRef<Set<string>>(new Set());
  const loadedRef = useRef<Set<string>>(new Set());
  const abortersRef = useRef<Map<string, AbortController>>(new Map());
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
    return () => {
      mountedRef.current = false;
      for (const controller of aborters.values()) controller.abort();
      aborters.clear();
      inFlight.clear();
      loaded.clear();
    };
  }, []);

  const ensure = useCallback((url: string) => {
    if (!url) return;
    if (inFlightRef.current.has(url) || loadedRef.current.has(url)) return;
    const parsed = parseGitHubRepoURL(url);
    if (!parsed) {
      loadedRef.current.add(url);
      setState((prev) => ({
        ...prev,
        [url]: { branches: [], loading: false, loaded: true },
      }));
      return;
    }
    setState((prev) => ({
      ...prev,
      [url]: {
        branches: prev[url]?.branches ?? [],
        loading: true,
        loaded: false,
      },
    }));
    inFlightRef.current.add(url);
    const controller = new AbortController();
    abortersRef.current.set(url, controller);

    fetchRepoBranches(parsed.owner, parsed.repo, {
      init: { signal: controller.signal },
    })
      .then((res) => {
        if (!mountedRef.current) return;
        const branches: Branch[] = (res?.branches ?? []).map((b) => ({
          name: b.name,
          type: "remote" as const,
        }));
        loadedRef.current.add(url);
        setState((prev) => ({
          ...prev,
          [url]: { branches, loading: false, loaded: true },
        }));
      })
      .catch(() => {
        if (!mountedRef.current) return;
        // On failure mark loaded so we don't retry in a tight loop. Callers
        // that need to retry can clear the entry and call ensure() again.
        loadedRef.current.add(url);
        setState((prev) => ({
          ...prev,
          [url]: {
            branches: prev[url]?.branches ?? [],
            loading: false,
            loaded: true,
          },
        }));
      })
      .finally(() => {
        inFlightRef.current.delete(url);
        abortersRef.current.delete(url);
      });
  }, []);

  const branches = useCallback((url: string): Branch[] => state[url]?.branches ?? EMPTY, [state]);
  const loading = useCallback((url: string): boolean => Boolean(state[url]?.loading), [state]);

  return { branches, loading, ensure };
}
