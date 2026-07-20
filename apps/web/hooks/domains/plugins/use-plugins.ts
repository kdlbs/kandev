"use client";

import { useQuery } from "@tanstack/react-query";
import { pluginsQueryOptions } from "@/lib/query/query-options";

function pluginErrorMessage(error: unknown): string | null {
  if (!error) return null;
  return error instanceof Error ? error.message : String(error);
}

/**
 * Reads the installed plugin registry from the shared query cache.
 */
export function usePlugins() {
  const query = useQuery(pluginsQueryOptions());
  return {
    items: query.data ?? [],
    loaded: query.isFetched,
    loading: query.isLoading,
    error: pluginErrorMessage(query.error),
  };
}
