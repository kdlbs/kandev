import { updateUserSettings } from "@/lib/api/domains/settings-api";
import { setUserSettingsSyncFailure } from "@/lib/user-settings-sync-failure";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

export function createQueuedUserSettingsSync<T>(
  syncFailedKey: string,
  buildPayload: (value: T) => UserSettingsUpdatePayload,
): (value: T) => Promise<void> {
  let queue = Promise.resolve();
  return (value: T) => {
    const payload = buildPayload(value);
    queue = queue
      .catch(() => undefined)
      .then(() =>
        updateUserSettings(payload)
          .then(() => {
            setUserSettingsSyncFailure(syncFailedKey, false);
          })
          .catch(() => {
            setUserSettingsSyncFailure(syncFailedKey, true);
          }),
      );
    return queue;
  };
}
