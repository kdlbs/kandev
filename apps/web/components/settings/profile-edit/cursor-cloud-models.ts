import type { CursorCloudConfigResponse } from "@/lib/api/domains/cursor-cloud-api";
import type { Executor, ModelEntry } from "@/lib/types/http";

// i18n-exempt: stable executor identity from the backend contract
const CURSOR_CLOUD_EXECUTOR_TYPE = "cursor_cloud";
// i18n-exempt: stable executor profile configuration key
const CURSOR_CLOUD_SECRET_ID = "cursor_cloud_api_key_secret_id";
// i18n-exempt: stable executor profile configuration key
const CURSOR_CLOUD_CALLBACK_URL = "cursor_cloud_callback_url";

export function cursorCloudProfiles(executors: Executor[]) {
  return executors
    .filter((executor) => executor.type === CURSOR_CLOUD_EXECUTOR_TYPE)
    .flatMap((executor) => executor.profiles ?? [])
    .flatMap((profile) => {
      const secretId = profile.config?.[CURSOR_CLOUD_SECRET_ID]?.trim();
      const callbackUrl = profile.config?.[CURSOR_CLOUD_CALLBACK_URL]?.trim();
      return secretId && callbackUrl ? [{ secretId, callbackUrl }] : [];
    });
}

export function cursorCloudModelEntries(catalogs: CursorCloudConfigResponse[]): ModelEntry[] {
  const models = new Map<string, ModelEntry>();
  for (const model of catalogs.flatMap((catalog) => catalog.models ?? [])) {
    if (models.has(model.id)) continue;
    models.set(model.id, {
      id: model.id,
      name: model.displayName,
      description: model.description,
      source: "static",
    });
  }
  return [...models.values()];
}
