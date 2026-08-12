import type { ApiClient } from "./api-client";
import type {
  MessageQueueSettingsPatch,
  MessageQueueSettingsResponse,
  MessageQueueSettingsValue,
} from "../../lib/types/system";

type KandevTest = typeof import("../fixtures/test-base").test;

export const MESSAGE_QUEUE_SETTINGS_PATH = "/api/v1/system/message-queue/settings";

export async function requestMessageQueueSettings(
  apiClient: ApiClient,
  method: "GET" | "PATCH",
  patch?: MessageQueueSettingsPatch,
): Promise<MessageQueueSettingsResponse> {
  const response = await apiClient.rawRequest(method, MESSAGE_QUEUE_SETTINGS_PATH, patch);
  if (!response.ok) {
    throw new Error(
      `${method} ${MESSAGE_QUEUE_SETTINGS_PATH} failed (${response.status}): ${await response.text()}`,
    );
  }
  return response.json() as Promise<MessageQueueSettingsResponse>;
}

export async function restoreMessageQueueSettings(
  apiClient: ApiClient,
  baseline: MessageQueueSettingsValue,
): Promise<void> {
  await requestMessageQueueSettings(apiClient, "PATCH", baseline);
}

/**
 * Register per-test isolation for specs whose assertions require separate
 * compatible queue rows. Captures and restores every install-wide queue field
 * so the helper cannot leak a concurrent capacity or manual-merge baseline.
 */
export function registerSeparateQueueRows(test: KandevTest): void {
  let baseline: MessageQueueSettingsValue | undefined;

  test.beforeEach(async ({ apiClient }) => {
    baseline = (await requestMessageQueueSettings(apiClient, "GET")).settings;
    await requestMessageQueueSettings(apiClient, "PATCH", { auto_merge_enabled: false });
  });

  test.afterEach(async ({ apiClient }) => {
    if (!baseline) return;
    await restoreMessageQueueSettings(apiClient, baseline);
    baseline = undefined;
  });
}
