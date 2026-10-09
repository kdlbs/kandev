/** Owns delayed simulation work so stop, deletion, and reset cancel every callback. */
export function createDemoRunTimers() {
  const sessions = new Map<string, Set<ReturnType<typeof setTimeout>>>();
  function cancel(sessionId: string) {
    for (const timer of sessions.get(sessionId) ?? []) clearTimeout(timer);
    sessions.delete(sessionId);
  }
  return {
    schedule(sessionId: string, callback: () => void, delay: number) {
      const timers = sessions.get(sessionId) ?? new Set();
      const timer = setTimeout(() => {
        timers.delete(timer);
        if (timers.size === 0) sessions.delete(sessionId);
        callback();
      }, delay);
      timers.add(timer);
      sessions.set(sessionId, timers);
    },
    cancel,
    clear() {
      for (const sessionId of sessions.keys()) cancel(sessionId);
    },
  };
}
