"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import type { LinearIssue } from "@/lib/types/linear";
import { linearIssuesQueryOptions } from "@/lib/query/query-options";

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
  const [page, setPage] = useState(1);
  // tokens[i] is the page_token for page i+1; tokens[0] is always "".
  const tokensRef = useRef<string[]>([""]);
  const [filters, setFilters] = useState({ query, teamKey, assigned });
  const searchQuery = useQuery({
    ...linearIssuesQueryOptions(workspaceId, {
      query: filters.query || undefined,
      teamKey: filters.teamKey || undefined,
      assigned: filters.assigned || undefined,
      pageToken: tokensRef.current[page - 1] || undefined,
      maxResults: LINEAR_PAGE_SIZE,
    }),
    enabled: Boolean(workspaceId) && enabled,
    placeholderData: keepPreviousData,
  });

  // Reset cursor stack and snap back to page 1 when filters change.
  useEffect(() => {
    tokensRef.current = [""];
    setPage(1);
  }, [workspaceId, query, teamKey, assigned]);

  useEffect(() => {
    const timeout = setTimeout(() => setFilters({ query, teamKey, assigned }), 250);
    return () => clearTimeout(timeout);
  }, [query, teamKey, assigned]);

  const result = searchQuery.data;
  const isLast = result?.isLast ?? true;

  return {
    items: result?.issues ?? [],
    loading: searchQuery.isFetching,
    error: searchQuery.error instanceof Error ? searchQuery.error.message : null,
    page,
    pageSize: LINEAR_PAGE_SIZE,
    isLast,
    goNext: () => {
      if (!isLast && result?.nextPageToken) {
        tokensRef.current[page] = result.nextPageToken;
        setPage((current) => current + 1);
      }
    },
    goPrev: () => {
      setPage((p) => Math.max(1, p - 1));
    },
  };
}
