import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useState } from "react";
import { officeTaskSearchQueryOptions } from "@/lib/query/query-options";
import type { OfficeTask } from "@/lib/state/slices/office/types";

const DEBOUNCE_MS = 300;

/**
 * Manages server-side task search with debounce.
 * Returns current search results (null when no active search) and
 * a handler to trigger searches.
 */
export function useServerSearch(workspaceId: string | null) {
  const [searchInput, setSearchInput] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  const search = useCallback((query: string) => {
    setSearchInput(query);
  }, []);

  useEffect(() => {
    const normalizedSearch = searchInput.trim();
    if (!normalizedSearch || !workspaceId) {
      setDebouncedSearch("");
      return;
    }
    const timeout = setTimeout(() => {
      setDebouncedSearch(normalizedSearch);
    }, DEBOUNCE_MS);
    return () => {
      clearTimeout(timeout);
    };
  }, [searchInput, workspaceId]);

  const searchQuery = useQuery(officeTaskSearchQueryOptions(workspaceId ?? "", debouncedSearch));
  const isActiveSearch = Boolean(workspaceId && debouncedSearch);
  const results: OfficeTask[] | null =
    isActiveSearch && !searchQuery.isError ? (searchQuery.data?.tasks ?? []) : null;

  return { searchResults: results, triggerSearch: search };
}
