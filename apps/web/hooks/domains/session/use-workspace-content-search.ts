"use client";

import { useEffect, useState } from "react";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceContent } from "@/lib/ws/workspace-files";

const CONTENT_SEARCH_DEBOUNCE_MS = 250;
const CONTENT_SEARCH_LIMIT_PER_REPO = 50;
const MAX_CONTENT_SEARCH_CODE_POINTS = 200;

export type WorkspaceContentSearchError =
  | "session-unavailable"
  | "query-too-long"
  | "transport-error";

type WorkspaceContentSearchOptions = {
  enabled: boolean;
  query: string;
  sessionId: string | null;
};

export function useWorkspaceContentSearch({
  enabled,
  query,
  sessionId,
}: WorkspaceContentSearchOptions) {
  const [results, setResults] = useState<WorkspaceContentSearchResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [error, setError] = useState<WorkspaceContentSearchError | null>(null);

  useEffect(() => {
    const normalizedQuery = query.trim();
    if (!enabled) {
      setResults([]);
      setIsSearching(false);
      setError(null);
      return;
    }
    if (!sessionId) {
      setResults([]);
      setIsSearching(false);
      setError("session-unavailable");
      return;
    }
    if (!normalizedQuery) {
      setResults([]);
      setIsSearching(false);
      setError(null);
      return;
    }
    if (Array.from(normalizedQuery).length > MAX_CONTENT_SEARCH_CODE_POINTS) {
      setResults([]);
      setIsSearching(false);
      setError("query-too-long");
      return;
    }

    let cancelled = false;
    setResults([]);
    setIsSearching(true);
    setError(null);
    const timer = setTimeout(async () => {
      const client = getWebSocketClient();
      if (!client) {
        if (!cancelled) {
          setResults([]);
          setError("transport-error");
          setIsSearching(false);
        }
        return;
      }

      try {
        const response = await searchWorkspaceContent(
          client,
          sessionId,
          normalizedQuery,
          CONTENT_SEARCH_LIMIT_PER_REPO,
        );
        if (!cancelled) {
          setResults(response.results ?? []);
          setError(null);
        }
      } catch {
        if (!cancelled) {
          setResults([]);
          setError("transport-error");
        }
      } finally {
        if (!cancelled) setIsSearching(false);
      }
    }, CONTENT_SEARCH_DEBOUNCE_MS);

    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [enabled, query, sessionId]);

  return { results, isSearching, error };
}
