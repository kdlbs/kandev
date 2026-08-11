import type { ApiClient } from "./api-client";

export type AppStatusBarOrderApi = {
  left_item_ids: string[];
  right_item_ids: string[];
};

export type AppStatusBarSettingsBaseline = {
  enabled: boolean;
  order?: AppStatusBarOrderApi;
};

export async function captureAppStatusBarSettings(
  apiClient: ApiClient,
): Promise<AppStatusBarSettingsBaseline> {
  const { settings } = await apiClient.getUserSettings();
  return {
    enabled: settings.app_status_bar_enabled !== false,
    order: settings.app_status_bar_order as AppStatusBarOrderApi | undefined,
  };
}

export async function setAppStatusBarEnabled(
  apiClient: ApiClient,
  enabled: boolean,
): Promise<void> {
  const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    app_status_bar_enabled: enabled,
  });
  if (!response.ok) throw new Error(`status bar preference update failed: ${response.status}`);
}

export async function restoreAppStatusBarSettings(
  apiClient: ApiClient,
  baseline: AppStatusBarSettingsBaseline | undefined,
): Promise<void> {
  if (!baseline) return;
  const payload: Record<string, unknown> = { app_status_bar_enabled: baseline.enabled };
  if (baseline.order) payload.app_status_bar_order = baseline.order;
  const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", payload);
  if (!response.ok) throw new Error(`status bar preference restore failed: ${response.status}`);
}
