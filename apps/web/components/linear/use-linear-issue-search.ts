"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { searchLinearIssues } from "@/lib/api/domains/linear-api";
import type { LinearIssue } from "@/lib/types/linear";

export const LINEAR_PAGE_SIZE = 25;

export type LinearSearchState = {
  items: LinearIssue[];
  loading: boolean;
  error: string | null;
  page: number;
  pageSize: number;
  isLast: boolean;
  goNext: () => void;
  goPrev: () => void;
};

// useLinearIssueSearch is page-based: it caches each page's cursor in
// `tokensRef` so the user can step backward without re-querying from the first
// page. Linear returns no total count, so the UI shows a row range plus a
// Page N indicator.
//
// `enabled` gates the network fetch on the integration being configured and
// available. When Linear is not configured the page renders the connect notice
// instead of the list, so firing the search would only produce failing (503)
// requests in the console.
export function useLinearIssueSearch(
  workspaceId: string | undefined,
  query: string,
  teamKey: string,
  assigned: string,
  enabled: boolean,
): LinearSearchState {
  const [items, setItems] = useState<LinearIssue[]>([]);
  const [page, setPage] = useState(1);
  const [isLast, setIsLast] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // tokens[i] is the page_token for page i+1; tokens[0] is always "".
  const tokensRef = useRef<string[]>([""]);
  const reqRef = useRef(0);

  const fetchPage = useCallback(
    (p: number) =>
      searchLinearIssues(
        {
          query: query || undefined,
          teamKey: teamKey || undefined,
          assigned: assigned || undefined,
          pageToken: tokensRef.current[p - 1] || undefined,
          maxResults: LINEAR_PAGE_SIZE,
        },
        { workspaceId },
      ),
    [workspaceId, query, teamKey, assigned],
  );

  const run = useCallback(
    async (p: number) => {
      if (!workspaceId || !enabled) return;
      const reqId = ++reqRef.current;
      setLoading(true);
      setError(null);
      try {
        const res = await fetchPage(p);
        if (reqId !== reqRef.current) return;
        setItems(res.issues ?? []);
        setIsLast(res.isLast ?? true);
        if (!res.isLast && res.nextPageToken) {
          tokensRef.current[p] = res.nextPageToken;
        }
      } catch (err) {
        if (reqId !== reqRef.current) return;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (reqId === reqRef.current) setLoading(false);
      }
    },
    [workspaceId, enabled, fetchPage],
  );

  // Reset cursor stack and snap back to page 1 when filters change.
  useEffect(() => {
    tokensRef.current = [""];
    setPage(1);
  }, [workspaceId, query, teamKey, assigned]);

  // 250ms debounce keeps the keyboard input from firing a request per char.
  useEffect(() => {
    if (!workspaceId || !enabled) return;
    const id = setTimeout(() => void run(page), 250);
    return () => clearTimeout(id);
  }, [run, page, workspaceId, enabled]);

  return {
    items,
    loading,
    error,
    page,
    pageSize: LINEAR_PAGE_SIZE,
    isLast,
    goNext: () => {
      if (!isLast) setPage((p) => p + 1);
    },
    goPrev: () => {
      setPage((p) => Math.max(1, p - 1));
    },
  };
}
