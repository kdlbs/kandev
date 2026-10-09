type RefreshEntry = {
  timer?: ReturnType<typeof setTimeout>;
  dueAt: number;
  refresh: () => void;
};

/** Schedules only subscribed scopes while the document is visible. */
export function createRemoteExecutorStatusRefresh(interval: number) {
  const active = new Map<string, RefreshEntry>();

  function schedule(entry: RefreshEntry) {
    clearTimeout(entry.timer);
    if (typeof document === "undefined" || document.visibilityState === "hidden") return;
    entry.timer = setTimeout(entry.refresh, Math.max(0, entry.dueAt - Date.now()));
  }

  function onVisibilityChange() {
    active.forEach(schedule);
  }

  return {
    activate(scope: string, refresh: () => void, dueAt: number) {
      if (active.has(scope)) return;
      if (active.size === 0 && typeof document !== "undefined") {
        document.addEventListener("visibilitychange", onVisibilityChange);
      }
      const entry = { refresh, dueAt };
      active.set(scope, entry);
      schedule(entry);
    },
    settled(scope: string) {
      const entry = active.get(scope);
      if (!entry) return;
      entry.dueAt = Date.now() + interval;
      schedule(entry);
    },
    deactivate(scope: string) {
      clearTimeout(active.get(scope)?.timer);
      active.delete(scope);
      if (active.size === 0 && typeof document !== "undefined") {
        document.removeEventListener("visibilitychange", onVisibilityChange);
      }
    },
  };
}
