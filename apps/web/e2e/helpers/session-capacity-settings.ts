import type { ApiClient } from "./api-client";
import type {
  SessionCapacitySettingsPatch,
  SessionCapacitySettingsResponse,
  SessionCapacitySettingsValue,
} from "../../lib/types/system";

export const SESSION_CAPACITY_SETTINGS_PATH = "/api/v1/system/session-capacity/settings";

export async function requestSessionCapacitySettings(
  apiClient: ApiClient,
  method: "GET" | "PATCH",
  patch?: SessionCapacitySettingsPatch,
): Promise<SessionCapacitySettingsResponse> {
  const response = await apiClient.rawRequest(method, SESSION_CAPACITY_SETTINGS_PATH, patch);
  if (!response.ok) {
    throw new Error(
      `${method} ${SESSION_CAPACITY_SETTINGS_PATH} failed (${response.status}): ${await response.text()}`,
    );
  }
  return response.json() as Promise<SessionCapacitySettingsResponse>;
}

export async function restoreSessionCapacitySettings(
  apiClient: ApiClient,
  baseline: SessionCapacitySettingsValue,
): Promise<void> {
  const current = await requestSessionCapacitySettings(apiClient, "GET");
  if (current.effective.locked) return;
  await requestSessionCapacitySettings(apiClient, "PATCH", {
    enabled: baseline.enabled,
    max_sessions: baseline.max_sessions,
  });
}
