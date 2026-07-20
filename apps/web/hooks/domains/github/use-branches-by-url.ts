"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchRepoBranches } from "@/lib/api/domains/github-api";
import { listProjectBranches } from "@/lib/api/domains/gitlab-api";
import { listAzureDevOpsBranches } from "@/lib/api/domains/azure-devops-api";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
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
  ensure: (url: string, workspaceId?: string) => void;
  /**
   * Forget the cached entry for `url` so the next `ensure(url)` re-fetches.
   * Aborts any in-flight request and discards any pending callbacks via the
   * per-URL sequence counter. Use after a failed fetch to retry.
   */
  clear: (url: string) => void;
};

const EMPTY: Branch[] = [];

/** Shared ref bag passed to the extracted handlers. Bundling the refs into
 *  one object keeps every handler's signature small and lets us split the
 *  fetch flow into focused steps without re-deriving the closure each call. */
type Refs = {
  mountedRef: React.MutableRefObject<boolean>;
  inFlightRef: React.MutableRefObject<Set<string>>;
  loadedRef: React.MutableRefObject<Set<string>>;
  abortersRef: React.MutableRefObject<Map<string, AbortController>>;
  seqRef: React.MutableRefObject<Map<string, number>>;
};

type SetState = React.Dispatch<React.SetStateAction<Record<string, URLState>>>;

/** Marks the entry as loading=true (preserving any prior branches) and bumps
 *  the per-URL sequence counter. Returns the new sequence number + the abort
 *  controller the fetch should hand to fetchRepoBranches. */
function initRequest(
  refs: Refs,
  setState: SetState,
  url: string,
): { seq: number; signal: AbortSignal } {
  setState((prev) => ({
    ...prev,
    [url]: { branches: prev[url]?.branches ?? [], loading: true },
  }));
  refs.inFlightRef.current.add(url);
  const controller = new AbortController();
  refs.abortersRef.current.set(url, controller);
  const seq = (refs.seqRef.current.get(url) ?? 0) + 1;
  refs.seqRef.current.set(url, seq);
  return { seq, signal: controller.signal };
}

/** Writes the successful branches list when the request is still current. */
function handleSuccess(
  refs: Refs,
  setState: SetState,
  url: string,
  seq: number,
  res: { branches?: Array<{ name: string }> },
): void {
  if (!refs.mountedRef.current) return;
  if (refs.seqRef.current.get(url) !== seq) return;
  const branches: Branch[] = (res?.branches ?? []).map((b) => ({
    name: b.name,
    type: "remote" as const,
  }));
  refs.loadedRef.current.add(url);
  setState((prev) => ({ ...prev, [url]: { branches, loading: false } }));
}

/** Marks the URL as no longer loading on failure. Does NOT add to loadedRef:
 *  leaving it unmarked lets the next ensure() call retry instead of
 *  short-circuiting on the cached failure. */
function handleFailure(refs: Refs, setState: SetState, url: string, seq: number): void {
  if (!refs.mountedRef.current) return;
  if (refs.seqRef.current.get(url) !== seq) return;
  setState((prev) => ({
    ...prev,
    [url]: { branches: prev[url]?.branches ?? [], loading: false },
  }));
}

/** Cleans up the in-flight + aborters maps for the request that just settled.
 *  Skipped when a newer request has superseded this one (it owns the slot). */
function finalizeRequest(refs: Refs, url: string, seq: number): void {
  if (refs.seqRef.current.get(url) !== seq) return;
  refs.inFlightRef.current.delete(url);
  refs.abortersRef.current.delete(url);
}

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
  const seqRef = useRef<Map<string, number>>(new Map());
  const mountedRef = useRef(true);
  const refsRef = useRef<Refs>({
    mountedRef,
    inFlightRef,
    loadedRef,
    abortersRef,
    seqRef,
  });

  useEffect(() => {
    mountedRef.current = true;
    // Snapshot refs into locals so the cleanup reads from the same object
    // identity the effect captured at mount (silences exhaustive-deps without
    // changing behavior — these refs are never reassigned).
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

  const ensure = useCallback((rawUrl: string, workspaceId: string = "") => {
    // Normalize on entry so the cache key is canonical — see the matching
    // comment in usePRInfoByURL for the rationale.
    const url = rawUrl.trim();
    if (!url) return;
    if (inFlightRef.current.has(url) || loadedRef.current.has(url)) return;
    // Accept plain repo URLs plus PR/issue URLs — branches are listed against
    // the repo in every case, so we extract just `{ owner, repo }` and ignore
    // the item number. Using the repo-only parser here used to reject PR URLs
    // outright, leaving the branch picker permanently empty when the user
    // pasted a PR link into the Remote tab.
    const request = branchRequestForURL(url, workspaceId);
    if (!request) {
      loadedRef.current.add(url);
      setState((prev) => ({ ...prev, [url]: { branches: [], loading: false } }));
      return;
    }
    const refs = refsRef.current;
    const { seq, signal } = initRequest(refs, setState, url);
    request(signal)
      .then((res) => handleSuccess(refs, setState, url, seq, res))
      .catch(() => handleFailure(refs, setState, url, seq))
      .finally(() => finalizeRequest(refs, url, seq));
  }, []);

  const clear = useCallback((rawUrl: string) => {
    const url = rawUrl.trim();
    if (!url) return;
    inFlightRef.current.delete(url);
    loadedRef.current.delete(url);
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

  const branches = useCallback(
    (rawUrl: string): Branch[] => state[rawUrl.trim()]?.branches ?? EMPTY,
    [state],
  );
  const loading = useCallback(
    (rawUrl: string): boolean => Boolean(state[rawUrl.trim()]?.loading),
    [state],
  );

  return { branches, loading, ensure, clear };
}

type BranchRequest = (signal: AbortSignal) => Promise<{ branches?: Array<{ name: string }> }>;

function branchRequestForURL(rawURL: string, workspaceId: string): BranchRequest | null {
  const github = parseGitHubAnyUrl(rawURL);
  if (github) {
    return (signal) => fetchRepoBranches(github.owner, github.repo, { init: { signal } });
  }
  const parsed = parseRemoteURL(rawURL);
  if (!parsed) return null;
  if (parsed.hostname === "github.com") {
    const parts = parsed.pathname
      .replace(/^\//, "")
      .replace(/\.git$/, "")
      .split("/");
    if (parts.length >= 2) {
      return (signal) => fetchRepoBranches(parts[0], parts[1], { init: { signal } });
    }
  }
  if (parsed.hostname === "gitlab.com") {
    const project = parsed.pathname.replace(/^\//, "").replace(/\.git$/, "");
    return (signal) => listProjectBranches(project, { init: { signal } });
  }
  if (parsed.hostname === "dev.azure.com" && workspaceId) {
    const parts = parsed.pathname.split("/").filter(Boolean);
    if (parts.length === 4 && parts[2] === "_git") {
      return (signal) =>
        listAzureDevOpsBranches(workspaceId, parts[1], parts[3], { init: { signal } });
    }
  }
  if (parsed.hostname === "ssh.dev.azure.com" && workspaceId) {
    const parts = parsed.pathname.split("/").filter(Boolean);
    if (parts.length === 4 && parts[0] === "v3") {
      return (signal) =>
        listAzureDevOpsBranches(workspaceId, parts[2], parts[3], { init: { signal } });
    }
  }
  return null;
}

function parseRemoteURL(raw: string): URL | null {
  let candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(raw) ? raw : `https://${raw}`;
  const scpMatch = raw.match(/^git@([^:]+):(.+)$/i);
  if (scpMatch) candidate = `ssh://git@${scpMatch[1]}/${scpMatch[2]}`;
  try {
    return new URL(candidate);
  } catch {
    return null;
  }
}
