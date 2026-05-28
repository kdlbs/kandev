"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

/**
 * Module-cached fetch for remote-auth specs.
 * Specs are static at runtime — staleTime: Infinity ensures one fetch per session.
 * `loaded` lets callers defer gating until the catalog is known.
 */
export function useRemoteAuthSpecs() {
  const query = useQuery(settingsQueryOptions.remoteAuthSpecs());
  return {
    specs: query.data ?? [],
    loaded: query.isSuccess,
  };
}
