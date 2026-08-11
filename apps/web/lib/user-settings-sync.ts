import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

const MAX_SYNC_ATTEMPTS = 3;
const BASE_SYNC_RETRY_DELAY_MS = 100;

async function requestUserSettingsUpdateWithRetry(
  payload: UserSettingsUpdatePayload,
): Promise<Awaited<ReturnType<typeof updateUserSettings>>> {
  let lastError: unknown;
  for (let attempt = 0; attempt < MAX_SYNC_ATTEMPTS; attempt += 1) {
    try {
      return await updateUserSettings(payload);
    } catch (error) {
      if (error instanceof ApiError) throw error;
      lastError = error;
      if (attempt < MAX_SYNC_ATTEMPTS - 1) {
        await new Promise((resolve) =>
          setTimeout(resolve, BASE_SYNC_RETRY_DELAY_MS * 2 ** attempt),
        );
      }
    }
  }
  throw lastError;
}

export async function updateUserSettingsWithRetry(
  payload: UserSettingsUpdatePayload,
): Promise<void> {
  await requestUserSettingsUpdateWithRetry(payload);
}

export function createQueuedUserSettingsSyncWithResponse<T>(
  buildPayload: (value: T) => UserSettingsUpdatePayload,
): (value: T) => ReturnType<typeof updateUserSettings> {
  let queue: Promise<unknown> = Promise.resolve();
  return (value: T) => {
    const payload = buildPayload(value);
    const response = queue
      .catch(() => undefined)
      .then(() => requestUserSettingsUpdateWithRetry(payload));
    queue = response;
    return response;
  };
}

export function createQueuedUserSettingsSync<T>(
  buildPayload: (value: T) => UserSettingsUpdatePayload,
): (value: T) => Promise<void> {
  const syncWithResponse = createQueuedUserSettingsSyncWithResponse(buildPayload);
  return async (value: T) => {
    await syncWithResponse(value);
  };
}
