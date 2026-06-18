export function hasUserSettingsSyncFailure(storageKey: string): boolean {
  if (typeof window === "undefined") return false;
  return window.localStorage.getItem(storageKey) === "1";
}

export function setUserSettingsSyncFailure(storageKey: string, failed: boolean): void {
  if (typeof window === "undefined") return;
  try {
    if (failed) window.localStorage.setItem(storageKey, "1");
    else window.localStorage.removeItem(storageKey);
  } catch {
    /* ignore storage failures */
  }
}
