"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { searchJiraTickets } from "@/lib/api/domains/jira-api";
import type { JiraTicket } from "@/lib/types/jira";

type SearchState = {
  items: JiraTicket[];
  loading: boolean;
  error: string | null;
  page: number;
  pageSize: number;
  isLast: boolean;
  lastFetchedAt: number | null;
  goNext: () => void;
  goPrev: () => void;
  refresh: () => void;
};

const PAGE_SIZE = 25;

// Atlassian's /search/jql is token-paginated: each response carries a
// nextPageToken cursor for the page that follows. We cache tokens for visited
// pages so users can step backward without re-querying from page 1.
export function useJiraSearch(workspaceId: string | null | undefined, jql: string): SearchState {
  const [items, setItems] = useState<JiraTicket[]>([]);
  const [page, setPage] = useState(1);
  const [isLast, setIsLast] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastFetchedAt, setLastFetchedAt] = useState<number | null>(null);
  // tokens[i] is the page_token for page i+1; tokens[0] is always "".
  const tokensRef = useRef<string[]>([""]);
  const reqRef = useRef(0);

  const run = useCallback(
    async (p: number) => {
      if (!workspaceId || !jql.trim()) return;
      const token = tokensRef.current[p - 1] ?? "";
      const reqId = ++reqRef.current;
      setLoading(true);
      setError(null);
      try {
        const res = await searchJiraTickets({
          jql,
          pageToken: token,
          maxResults: PAGE_SIZE,
        });
        if (reqId !== reqRef.current) return;
        setItems(res.tickets ?? []);
        setIsLast(res.isLast ?? true);
        if (!res.isLast && res.nextPageToken) {
          tokensRef.current[p] = res.nextPageToken;
        }
        setLastFetchedAt(Date.now());
      } catch (err) {
        if (reqId !== reqRef.current) return;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (reqId === reqRef.current) setLoading(false);
      }
    },
    [workspaceId, jql],
  );

  useEffect(() => {
    tokensRef.current = [""];
    setPage(1);
  }, [workspaceId, jql]);

  useEffect(() => {
    void run(page);
  }, [run, page]);

  return {
    items,
    loading,
    error,
    page,
    pageSize: PAGE_SIZE,
    isLast,
    lastFetchedAt,
    goNext: () => {
      if (!isLast) setPage((p) => p + 1);
    },
    goPrev: () => {
      setPage((p) => Math.max(1, p - 1));
    },
    refresh: () => run(page),
  };
}
