import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

const MAX_SYNC_ATTEMPTS = 3;

async function updateWithRetry(payload: UserSettingsUpdatePayload): Promise<void> {
  let lastError: unknown;
  for (let attempt = 0; attempt < MAX_SYNC_ATTEMPTS; attempt += 1) {
    try {
      await updateUserSettings(payload);
      return;
    } catch (error) {
      lastError = error;
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
      .then(() => updateWithRetry(payload))
      .catch(() => undefined);
    return queue;
  };
}
