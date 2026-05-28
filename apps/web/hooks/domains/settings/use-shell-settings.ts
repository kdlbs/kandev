"use client";

import { useQuery } from "@tanstack/react-query";
import { settingsQueryOptions } from "@/lib/query/query-options/settings";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";

export function useShellSettings() {
  const query = useQuery(settingsQueryOptions.userSettings());
  const mapped = query.data ? mapUserSettingsResponse(query.data) : null;

  return {
    preferredShell: mapped?.preferredShell ?? null,
    shellOptions: mapped?.shellOptions ?? [],
    loaded: query.isSuccess,
  };
}
