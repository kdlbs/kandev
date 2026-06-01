"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";

export function useShellSettings() {
  const query = useQuery(settingsQueryOptions.userSettings());

  return {
    preferredShell: query.data?.preferredShell ?? null,
    shellOptions: query.data?.shellOptions ?? [],
    loaded: query.isSuccess,
  };
}
