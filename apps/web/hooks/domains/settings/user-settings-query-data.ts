import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import type { UserSettingsState } from "@/lib/state/slices/settings/types";
import type { UserSettingsResponse } from "@/lib/types/http";

export type UserSettingsQueryData = UserSettingsResponse | UserSettingsState;

function isMappedUserSettings(data: UserSettingsQueryData): data is UserSettingsState {
  return "loaded" in data && "repositoryIds" in data;
}

export function mapUserSettingsQueryData(
  data: UserSettingsQueryData | null | undefined,
): UserSettingsState | null {
  if (!data) return null;
  if (isMappedUserSettings(data)) return data.loaded ? data : null;
  const mapped = mapUserSettingsResponse(data);
  return mapped.loaded ? mapped : null;
}
