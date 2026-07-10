const WALKTHROUGH_NOTIFICATION_KEY = "kandev.walkthrough.lastSeenByTask";

export type WalkthroughNotificationState = Record<string, string | null>;

function readState(): WalkthroughNotificationState {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(WALKTHROUGH_NOTIFICATION_KEY);
    return raw ? (JSON.parse(raw) as WalkthroughNotificationState) : {};
  } catch {
    return {};
  }
}

function writeState(state: WalkthroughNotificationState): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(WALKTHROUGH_NOTIFICATION_KEY, JSON.stringify(state));
  } catch {
    // Ignore write failures.
  }
}

export function getWalkthroughNotificationState(): WalkthroughNotificationState {
  return readState();
}

export function setWalkthroughLastSeen(taskId: string, timestamp: string | null): void {
  const state = readState();
  if (timestamp === null) {
    delete state[taskId];
  } else {
    state[taskId] = timestamp;
  }
  writeState(state);
}

export function getWalkthroughLastSeen(taskId: string): string | null {
  const state = readState();
  return state[taskId] ?? null;
}
