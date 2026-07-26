"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchIssueInfo, fetchPRInfo } from "@/lib/api/domains/github-api";
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
  prHeadBranch?: string;
  prBaseBranch?: string;
  prNumber?: number;
  issueNumber?: number;
  suggestedTitle: string;
};

type URLState = {
  info: PRInfo | undefined;
  loading: boolean;
  error?: Error;
};

export type UsePRInfoByURLResult = {
  ensure: (url: string) => void;
  info: (url: string) => PRInfo | undefined;
  loading: (url: string) => boolean;
  error: (url: string) => Error | undefined;
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
    /^(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/pull\/(\d+)(?:[/?#].*)?$/,
  );
  if (!prMatch) return null;
  return { owner: prMatch[1], repo: prMatch[2], prNumber: parseInt(prMatch[3], 10) };
}

/** Parse a GitHub URL and return the owner/repo/issueNumber when it's an
 * issue URL. Returns null for PR URLs, plain repos, invalid input. */
export function parseGitHubIssueUrl(
  url: string,
): { owner: string; repo: string; issueNumber: number } | null {
  const trimmed = url.trim();
  if (!trimmed) return null;
  const issueMatch = trimmed.match(
    /^(?:https?:\/\/)?(?:www\.)?github\.com\/([A-Za-z0-9_.-]+)\/([A-Za-z0-9_.-]+)\/issues\/(\d+)(?:[/?#].*)?$/,
  );
  if (!issueMatch) return null;
  return { owner: issueMatch[1], repo: issueMatch[2], issueNumber: parseInt(issueMatch[3], 10) };
}

/** Parse a GitHub URL as a PR URL, issue URL, or plain repo URL. Re-exported
 * so the legacy `parseGitHubUrl` shape (used elsewhere in the dialog code)
 * has a single canonical implementation. */
export function parseGitHubAnyUrl(
  url: string,
): { owner: string; repo: string; prNumber?: number; issueNumber?: number } | null {
  const pr = parseGitHubPrUrl(url);
  if (pr) return pr;
  const issue = parseGitHubIssueUrl(url);
  if (issue) return issue;
  return parseGitHubRepoUrl(url);
}

/** Shared ref bag passed to the extracted handlers. Bundling the refs into
 *  one object keeps every handler's signature small and lets us split the
 *  fetch flow into focused steps without re-deriving the closure each call. */
type Refs = {
  mountedRef: React.MutableRefObject<boolean>;
  workspaceIdRef: React.MutableRefObject<string | null>;
  workspaceEpochRef: React.MutableRefObject<number>;
  inFlightRef: React.MutableRefObject<Set<string>>;
  loadedRef: React.MutableRefObject<Set<string>>;
  abortersRef: React.MutableRefObject<Map<string, AbortController>>;
  seqRef: React.MutableRefObject<Map<string, number>>;
};

type RequestIdentity = {
  workspaceId: string | null;
  workspaceEpoch: number;
  sequence: number;
};

type SetState = React.Dispatch<React.SetStateAction<Record<string, URLState>>>;

type SuccessArgs<T> = {
  refs: Refs;
  setState: SetState;
  url: string;
  request: RequestIdentity;
  value: T;
  buildInfo: (value: T) => PRInfo;
};

/** Marks the entry as loading=true (preserving any prior info) and bumps the
 *  per-URL sequence counter. */
function initRequest(
  refs: Refs,
  setState: SetState,
  url: string,
): { request: RequestIdentity; signal: AbortSignal } {
  setState((prev) => ({
    ...prev,
    [url]: { info: prev[url]?.info, loading: true },
  }));
  refs.inFlightRef.current.add(url);
  const controller = new AbortController();
  refs.abortersRef.current.set(url, controller);
  const sequence = (refs.seqRef.current.get(url) ?? 0) + 1;
  refs.seqRef.current.set(url, sequence);
  return {
    request: {
      workspaceId: refs.workspaceIdRef.current,
      workspaceEpoch: refs.workspaceEpochRef.current,
      sequence,
    },
    signal: controller.signal,
  };
}

function isCurrentRequest(refs: Refs, url: string, request: RequestIdentity): boolean {
  return (
    refs.workspaceIdRef.current === request.workspaceId &&
    refs.workspaceEpochRef.current === request.workspaceEpoch &&
    refs.seqRef.current.get(url) === request.sequence
  );
}

/** Writes successful GitHub URL info when the request is still current. */
function handleSuccess<T>(args: SuccessArgs<T>): void {
  const { refs, setState, url, request, value, buildInfo } = args;
  if (!refs.mountedRef.current) return;
  if (!isCurrentRequest(refs, url, request)) return;
  refs.loadedRef.current.add(url);
  setState((prev) => ({ ...prev, [url]: { info: buildInfo(value), loading: false } }));
}

/** Marks loaded on failure (we don't want to retry in a tight loop) and
 *  clears the loading flag. Callers that want to retry can clear() + ensure(). */
function handleFailure(
  refs: Refs,
  setState: SetState,
  url: string,
  request: RequestIdentity,
  error: unknown,
): void {
  if (!refs.mountedRef.current) return;
  if (!isCurrentRequest(refs, url, request)) return;
  refs.loadedRef.current.add(url);
  setState((prev) => ({
    ...prev,
    [url]: {
      info: prev[url]?.info,
      loading: false,
      error: error instanceof Error ? error : new Error("Could not load GitHub metadata."),
    },
  }));
}

/** Cleans up the in-flight + aborters maps for the request that just settled. */
function finalizeRequest(refs: Refs, url: string, request: RequestIdentity): void {
  if (!isCurrentRequest(refs, url, request)) return;
  refs.inFlightRef.current.delete(url);
  refs.abortersRef.current.delete(url);
}

function runGitHubInfoRequest(args: {
  workspaceId: string;
  refs: Refs;
  setState: SetState;
  url: string;
  request: RequestIdentity;
  signal: AbortSignal;
  pr: NonNullable<ReturnType<typeof parseGitHubPrUrl>> | null;
  issue: ReturnType<typeof parseGitHubIssueUrl>;
}): void {
  const { workspaceId, refs, setState, url, request, signal, pr, issue } = args;
  let pendingRequest: Promise<void>;
  if (pr) {
    pendingRequest = fetchPRInfo(workspaceId, pr.owner, pr.repo, pr.prNumber, {
      init: { signal },
    }).then((res) =>
      handleSuccess({
        refs,
        setState,
        url,
        request,
        value: res,
        buildInfo: (value) => ({
          prHeadBranch: value.head_branch,
          prBaseBranch: value.base_branch,
          prNumber: value.number,
          suggestedTitle: `PR #${value.number}: ${value.title}`,
        }),
      }),
    );
  } else if (issue) {
    pendingRequest = fetchIssueInfo(workspaceId, issue.owner, issue.repo, issue.issueNumber, {
      init: { signal },
    }).then((res) =>
      handleSuccess({
        refs,
        setState,
        url,
        request,
        value: res,
        buildInfo: (value) => ({
          issueNumber: value.number,
          suggestedTitle: `Issue #${value.number}: ${value.title}`,
        }),
      }),
    );
  } else {
    return;
  }
  pendingRequest
    .catch((error) => handleFailure(refs, setState, url, request, error))
    .finally(() => finalizeRequest(refs, url, request));
}

export function usePRInfoByURL(workspaceId: string | null): UsePRInfoByURLResult {
  const [state, setState] = useState<Record<string, URLState>>({});
  const inFlightRef = useRef<Set<string>>(new Set());
  const loadedRef = useRef<Set<string>>(new Set());
  const abortersRef = useRef<Map<string, AbortController>>(new Map());
  const workspaceIdRef = useRef(workspaceId);
  const workspaceEpochRef = useRef(0);
  // Per-URL request sequence number. Incremented before each fetch so the
  // settled callbacks can confirm they're still the latest request for `url`.
  const seqRef = useRef<Map<string, number>>(new Map());
  const mountedRef = useRef(true);
  const refsRef = useRef<Refs>({
    mountedRef,
    workspaceIdRef,
    workspaceEpochRef,
    inFlightRef,
    loadedRef,
    abortersRef,
    seqRef,
  });

  useEffect(() => {
    mountedRef.current = true;
    const aborters = abortersRef.current;
    const inFlight = inFlightRef.current;
    const loaded = loadedRef.current;
    const seqs = seqRef.current;
    return () => {
      mountedRef.current = false;
      for (const controller of aborters.values()) controller.abort();
      aborters.clear();
      inFlight.clear();
      loaded.clear();
      seqs.clear();
    };
  }, []);

  useEffect(() => {
    workspaceIdRef.current = workspaceId;
    workspaceEpochRef.current += 1;
    for (const controller of abortersRef.current.values()) controller.abort();
    abortersRef.current.clear();
    inFlightRef.current.clear();
    loadedRef.current.clear();
    setState({});
  }, [workspaceId]);

  const ensure = useCallback(
    (rawUrl: string) => {
      // Normalize on entry so all internal state (in-flight, loaded, aborters,
      // sequence counter, state map) is keyed on the same canonical form.
      // Without this, a chip wiring that called ensure() with stray whitespace
      // could cache under the whitespaced key while consumers that look up
      // via the trimmed URL would miss the cache.
      const url = rawUrl.trim();
      if (!url || !workspaceId) return;
      if (inFlightRef.current.has(url) || loadedRef.current.has(url)) return;
      const pr = parseGitHubPrUrl(url);
      const issue = pr ? null : parseGitHubIssueUrl(url);
      if (!pr && !issue) {
        // Non-PR URLs (plain repo, invalid) are recorded as "loaded with no
        // info" so subsequent ensure() calls for the same URL no-op instead
        // of re-parsing on every call.
        loadedRef.current.add(url);
        return;
      }
      const refs = refsRef.current;
      const { request: requestIdentity, signal } = initRequest(refs, setState, url);
      runGitHubInfoRequest({
        workspaceId,
        refs,
        setState,
        url,
        request: requestIdentity,
        signal,
        pr,
        issue,
      });
    },
    [workspaceId],
  );

  const info = useCallback(
    (rawUrl: string): PRInfo | undefined => state[rawUrl.trim()]?.info,
    [state],
  );
  const loading = useCallback(
    (rawUrl: string): boolean => Boolean(state[rawUrl.trim()]?.loading),
    [state],
  );
  const error = useCallback(
    (rawUrl: string): Error | undefined => state[rawUrl.trim()]?.error,
    [state],
  );
  const clear = useCallback((rawUrl: string) => {
    const url = rawUrl.trim();
    if (!url) return;
    inFlightRef.current.delete(url);
    loadedRef.current.delete(url);
    // Bump the sequence so any in-flight callbacks for this URL bail —
    // they would otherwise resurrect the cleared state.
    seqRef.current.set(url, (seqRef.current.get(url) ?? 0) + 1);
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

  return { ensure, info, loading, error, clear };
}
