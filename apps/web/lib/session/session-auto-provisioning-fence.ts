const suppressedTaskIds = new Set<string>();
const storageKey = "kandev.bulk-session-removal-fences";

function readPersistedTaskIds(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const stored = JSON.parse(window.localStorage.getItem(storageKey) ?? "[]") as unknown;
    return Array.isArray(stored) ? stored.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

function replaceSuppressedTaskIds(taskIds: Iterable<string>) {
  suppressedTaskIds.clear();
  for (const taskId of taskIds) suppressedTaskIds.add(taskId);
}

function persist() {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(storageKey, JSON.stringify([...suppressedTaskIds]));
  } catch {
    // Persistence only preserves the fence across reloads; the current tab remains protected.
  }
}

function restore(taskId: string) {
  if (typeof window === "undefined" || suppressedTaskIds.has(taskId)) return;
  readPersistedTaskIds().forEach((id) => suppressedTaskIds.add(id));
}

export function suppressTaskSessionAutoProvisioning(taskId: string) {
  replaceSuppressedTaskIds([...suppressedTaskIds, ...readPersistedTaskIds(), taskId]);
  persist();
}

export function isTaskSessionAutoProvisioningSuppressed(taskId: string | null) {
  if (!taskId) return false;
  restore(taskId);
  return suppressedTaskIds.has(taskId);
}

export function clearTaskSessionAutoProvisioningSuppression(taskId: string) {
  const taskIds = new Set([...suppressedTaskIds, ...readPersistedTaskIds()]);
  taskIds.delete(taskId);
  replaceSuppressedTaskIds(taskIds);
  persist();
}
