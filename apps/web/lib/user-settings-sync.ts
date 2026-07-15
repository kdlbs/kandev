import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { ApiError } from "@/lib/api/client";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

const MAX_SYNC_ATTEMPTS = 3;
const BASE_SYNC_RETRY_DELAY_MS = 100;

export async function updateUserSettingsWithRetry(
  payload: UserSettingsUpdatePayload,
): Promise<void> {
  let lastError: unknown;
  for (let attempt = 0; attempt < MAX_SYNC_ATTEMPTS; attempt += 1) {
    try {
      await updateUserSettings(payload);
      return;
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

export function createQueuedUserSettingsSync<T>(
  buildPayload: (value: T) => UserSettingsUpdatePayload,
): (value: T) => Promise<void> {
  let queue = Promise.resolve();
  return (value: T) => {
    const payload = buildPayload(value);
    queue = queue
      .catch(() => undefined)
      .then(() => updateUserSettingsWithRetry(payload))
      .catch(() => undefined);
    return queue;
  };
}
