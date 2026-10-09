import { fetchJson, fetchJsonWithRetry, type ApiRequestOptions } from "../client";
import type {
  SessionCapacitySettingsPatch,
  SessionCapacitySettingsResponse,
} from "@/lib/types/system";

const SESSION_CAPACITY_SETTINGS_PATH = "/api/v1/system/session-capacity/settings";

/** Fetches the saved and effective automatic session capacity settings. */
export async function fetchSessionCapacitySettings(
  options?: ApiRequestOptions,
): Promise<SessionCapacitySettingsResponse> {
  return fetchJsonWithRetry<SessionCapacitySettingsResponse>(SESSION_CAPACITY_SETTINGS_PATH, {
    ...options,
    cache: "no-store",
  });
}

/** Applies a partial automatic session capacity update. */
export async function updateSessionCapacitySettings(
  payload: SessionCapacitySettingsPatch,
  options?: ApiRequestOptions,
): Promise<SessionCapacitySettingsResponse> {
  return fetchJson<SessionCapacitySettingsResponse>(SESSION_CAPACITY_SETTINGS_PATH, {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "PATCH",
      body: JSON.stringify(payload),
    },
  });
}
