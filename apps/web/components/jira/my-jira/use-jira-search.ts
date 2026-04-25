"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { searchJiraTickets } from "@/lib/api/domains/jira-api";
import type { JiraTicket } from "@/lib/types/jira";

type SearchState = {
  items: JiraTicket[];
  total: number;
  loading: boolean;
  error: string | null;
  page: number;
  pageSize: number;
  lastFetchedAt: number | null;
  setPage: (page: number) => void;
  refresh: () => void;
};

const PAGE_SIZE = 25;

export function useJiraSearch(workspaceId: string | null | undefined, jql: string): SearchState {
  const [items, setItems] = useState<JiraTicket[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [lastFetchedAt, setLastFetchedAt] = useState<number | null>(null);
  const reqRef = useRef(0);

  const run = useCallback(
    async (p: number) => {
      if (!workspaceId || !jql.trim()) return;
      const reqId = ++reqRef.current;
      setLoading(true);
      setError(null);
      try {
        const res = await searchJiraTickets(workspaceId, {
          jql,
          startAt: (p - 1) * PAGE_SIZE,
          maxResults: PAGE_SIZE,
        });
        if (reqId !== reqRef.current) return;
        setItems(res.tickets ?? []);
        setTotal(res.total ?? 0);
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
    setPage(1);
  }, [workspaceId, jql]);

  useEffect(() => {
    void run(page);
  }, [run, page]);

  return {
    items,
    total,
    loading,
    error,
    page,
    pageSize: PAGE_SIZE,
    lastFetchedAt,
    setPage,
    refresh: () => run(page),
  };
}
