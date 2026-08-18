"use client";

import { useEffect, useState } from "react";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";
import { getWebSocketClient } from "@/lib/ws/connection";
import { searchWorkspaceContent } from "@/lib/ws/workspace-files";

const CONTENT_SEARCH_DEBOUNCE_MS = 250;
const CONTENT_SEARCH_RETRY_ATTEMPTS = 8;
const CONTENT_SEARCH_RETRY_DELAY_MS = 500;
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

function contentSearchResultKey(result: WorkspaceContentSearchResult): string {
  return [result.repository_name, result.path, result.line, result.column].join("\u0000");
}

async function searchWorkspaceContentWithRetries(
  sessionId: string,
  query: string,
  isCancelled: () => boolean,
  onResults: (results: WorkspaceContentSearchResult[]) => void,
): Promise<WorkspaceContentSearchResult[]> {
  const client = getWebSocketClient();
  if (!client) throw new Error("websocket transport unavailable");

  const results = new Map<string, WorkspaceContentSearchResult>();
  let lastError: unknown;
  for (let attempt = 0; attempt < CONTENT_SEARCH_RETRY_ATTEMPTS; attempt += 1) {
    if (isCancelled()) return [];
    if (attempt > 0) {
      await new Promise((resolve) => setTimeout(resolve, CONTENT_SEARCH_RETRY_DELAY_MS));
      if (isCancelled()) return [];
    }
    try {
      const response = await searchWorkspaceContent(
        client,
        sessionId,
        query,
        CONTENT_SEARCH_LIMIT_PER_REPO,
      );
      for (const result of response.results ?? []) {
        results.set(contentSearchResultKey(result), result);
      }
      if (!isCancelled()) onResults([...results.values()]);
    } catch (error) {
      lastError = error;
    }
  }

  if (results.size === 0 && lastError) throw lastError;
  return [...results.values()];
}

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
      try {
        const nextResults = await searchWorkspaceContentWithRetries(
          sessionId,
          normalizedQuery,
          () => cancelled,
          (partialResults) => {
            if (!cancelled) {
              setResults(partialResults);
              setError(null);
            }
          },
        );
        if (!cancelled) {
          setResults(nextResults);
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
