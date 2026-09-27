const suppressedTaskIds = new Set<string>();
const storageKey = "kandev.bulk-session-removal-fences";

function persist() {
  if (typeof window !== "undefined")
    window.localStorage.setItem(storageKey, JSON.stringify([...suppressedTaskIds]));
}

function restore(taskId: string) {
  if (typeof window === "undefined" || suppressedTaskIds.has(taskId)) return;
  try {
    const stored = JSON.parse(window.localStorage.getItem(storageKey) ?? "[]") as string[];
    stored.forEach((id) => suppressedTaskIds.add(id));
  } catch {
    window.localStorage.removeItem(storageKey);
  }
}

export function suppressTaskSessionAutoProvisioning(taskId: string) {
  suppressedTaskIds.add(taskId);
  persist();
}

export function isTaskSessionAutoProvisioningSuppressed(taskId: string | null) {
  if (!taskId) return false;
  restore(taskId);
  return suppressedTaskIds.has(taskId);
}

export function clearTaskSessionAutoProvisioningSuppression(taskId: string) {
  suppressedTaskIds.delete(taskId);
  persist();
}
