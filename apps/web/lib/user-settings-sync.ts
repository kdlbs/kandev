import { updateUserSettings } from "@/lib/api/domains/settings-api";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

export function createQueuedUserSettingsSync<T>(
  buildPayload: (value: T) => UserSettingsUpdatePayload,
): (value: T) => Promise<void> {
  let queue = Promise.resolve();
  return (value: T) => {
    const payload = buildPayload(value);
    queue = queue
      .catch(() => undefined)
      .then(() => updateUserSettings(payload).then(() => undefined))
      .catch(() => undefined);
    return queue;
  };
}
