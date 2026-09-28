const storageKeyPrefix = "kandev.bulk-session-removal-fence.";
const unpersistedStateByTaskId = new Map<string, boolean>();

function persist(taskId: string, suppressed: boolean) {
  unpersistedStateByTaskId.set(taskId, suppressed);
  if (typeof window === "undefined") return;
  try {
    const key = `${storageKeyPrefix}${taskId}`;
    if (suppressed) window.localStorage.setItem(key, "1");
    else window.localStorage.removeItem(key);
    unpersistedStateByTaskId.delete(taskId);
  } catch {
    // Browser storage is best-effort; the current tab still honors the fence.
  }
}

export function suppressTaskSessionAutoProvisioning(taskId: string) {
  persist(taskId, true);
}

export function isTaskSessionAutoProvisioningSuppressed(taskId: string | null) {
  if (!taskId) return false;
  const unpersisted = unpersistedStateByTaskId.get(taskId);
  if (unpersisted !== undefined) return unpersisted;
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(`${storageKeyPrefix}${taskId}`) === "1";
  } catch {
    return false;
  }
}

export function clearTaskSessionAutoProvisioningSuppression(taskId: string) {
  persist(taskId, false);
}
