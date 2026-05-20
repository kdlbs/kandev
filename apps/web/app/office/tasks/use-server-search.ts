import { useCallback, useEffect, useRef, useState } from "react";
import { searchTasks } from "@/lib/api/domains/office-api";
import type { OfficeTask } from "@/lib/state/slices/office/types";

const DEBOUNCE_MS = 300;

/**
 * Manages server-side task search with debounce.
 * Returns current search results (null when no active search) and
 * a handler to trigger searches.
 */
export function useServerSearch(workspaceId: string | null) {
  const [results, setResults] = useState<OfficeTask[] | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const search = useCallback(
    (query: string) => {
      if (debounceRef.current) clearTimeout(debounceRef.current);

      if (!query.trim()) {
        setResults(null);
        return;
      }
      if (!workspaceId) return;

      debounceRef.current = setTimeout(() => {
        searchTasks(workspaceId, query)
          .then((res) => setResults(res.tasks ?? []))
          .catch(() => setResults(null));
      }, DEBOUNCE_MS);
    },
    [workspaceId],
  );

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  return { searchResults: results, triggerSearch: search };
}
