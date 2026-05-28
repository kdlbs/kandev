"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchPRInfo } from "@/lib/api/domains/github-api";
import { parseGitHubRepoUrl } from "@/lib/github/parse-url";

/**
 * Per-URL PR-info loader for GitHub PR URLs. Mirrors the shape of
 * `useBranchesByURL` so callers can hand a Remote-tab chip both hooks and
 * have it auto-select the PR head branch + surface the auto-fill title for
 * its own row without depending on dialog-level singletons.
 *
 * Behavior:
 *   - `ensure(url)` triggers a PR-info fetch the first time a PR URL is
 *     seen; non-PR URLs (plain repo URLs / invalid input / empty string)
 *     are no-ops.
 *   - Dedupes concurrent / repeat `ensure` calls per URL (mirrors the
 *     in-flight + loaded refs from `useBranchesByURL`).
 *   - `info(url)` returns the most-recently loaded PR info for `url`, or
 *     `undefined` if none has been loaded.
 *   - `loading(url)` returns true while a fetch for `url` is in flight.
 *   - `clear(url)` forgets the cached entry so the next `ensure` re-fetches.
 *
 * Per-URL state is scoped to the hook instance (not the module), so two
 * callers of this hook don't share cache. That mirrors how the dialog uses
 * the sibling `useBranchesByURL` hook today.
 */

export type PRInfo = {
  prHeadBranch: string;
  prBaseBranch: string;
  prNumber: number;
  suggestedTitle: string;
};

type URLState = {
  info: PRInfo | undefined;
  loading: boolean;
};

export type UsePRInfoByURLResult = {
  ensure: (url: string) => void;
  info: (url: string) => PRInfo | undefined;
  loading: (url: string) => boolean;
  clear: (url: string) => void;
};

/** Parse a GitHub URL and return the owner/repo/prNumber when it's a PR URL.
 * Returns null for non-PR URLs (plain repos, invalid input). The Remote-tab
 * flow uses this to decide whether to attempt a PR-info fetch at all. */
export function parseGitHubPrUrl(
  url: string,
): { owner: string; repo: string; prNumber: number } | null {
  const trimmed = url.trim();
  if (!trimmed) return null;
  const prMatch = trimmed.match(
    /(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/(\d+)(?:[/?#].*)?$/,
  );
  if (!prMatch) return null;
  return { owner: prMatch[1], repo: prMatch[2], prNumber: parseInt(prMatch[3], 10) };
}

/** Parse a GitHub URL as either a PR URL or a plain repo URL. Re-exported
 * so the legacy `parseGitHubUrl` shape (used elsewhere in the dialog code)
 * has a single canonical implementation. */
export function parseGitHubAnyUrl(
  url: string,
): { owner: string; repo: string; prNumber?: number } | null {
  const pr = parseGitHubPrUrl(url);
  if (pr) return pr;
  return parseGitHubRepoUrl(url);
}

export function usePRInfoByURL(): UsePRInfoByURLResult {
  const [state, setState] = useState<Record<string, URLState>>({});
  const inFlightRef = useRef<Set<string>>(new Set());
  const loadedRef = useRef<Set<string>>(new Set());
  const abortersRef = useRef<Map<string, AbortController>>(new Map());
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
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
    const parsed = parseGitHubPrUrl(url);
    if (!parsed) {
      // Non-PR URLs (plain repo, invalid) are recorded as "loaded with no
      // info" so subsequent ensure() calls for the same URL no-op instead
      // of re-parsing on every call.
      loadedRef.current.add(url);
      return;
    }
    setState((prev) => ({
      ...prev,
      [url]: { info: prev[url]?.info, loading: true },
    }));
    inFlightRef.current.add(url);
    const controller = new AbortController();
    abortersRef.current.set(url, controller);

    fetchPRInfo(parsed.owner, parsed.repo, parsed.prNumber, {
      init: { signal: controller.signal },
    })
      .then((pr) => {
        if (!mountedRef.current) return;
        loadedRef.current.add(url);
        const info: PRInfo = {
          prHeadBranch: pr.head_branch,
          prBaseBranch: pr.base_branch,
          prNumber: pr.number,
          suggestedTitle: `PR #${pr.number}: ${pr.title}`,
        };
        setState((prev) => ({
          ...prev,
          [url]: { info, loading: false },
        }));
      })
      .catch(() => {
        if (!mountedRef.current) return;
        // On failure mark loaded so we don't retry in a tight loop. Callers
        // that want to retry can clear() and ensure() again.
        loadedRef.current.add(url);
        setState((prev) => ({
          ...prev,
          [url]: { info: prev[url]?.info, loading: false },
        }));
      })
      .finally(() => {
        inFlightRef.current.delete(url);
        abortersRef.current.delete(url);
      });
  }, []);

  const info = useCallback((url: string): PRInfo | undefined => state[url]?.info, [state]);
  const loading = useCallback((url: string): boolean => Boolean(state[url]?.loading), [state]);
  const clear = useCallback((url: string) => {
    if (!url) return;
    inFlightRef.current.delete(url);
    loadedRef.current.delete(url);
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

  return { ensure, info, loading, clear };
}
