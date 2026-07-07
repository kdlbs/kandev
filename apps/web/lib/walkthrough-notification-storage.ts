import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";

const WALKTHROUGH_NOTIFICATION_KEY = "kandev.walkthrough.lastSeenByTask";

export type WalkthroughNotificationState = Record<string, string | null>;

export function getWalkthroughNotificationState(): WalkthroughNotificationState {
  return getLocalStorage(WALKTHROUGH_NOTIFICATION_KEY, {} as WalkthroughNotificationState);
}

export function setWalkthroughLastSeen(taskId: string, timestamp: string | null): void {
  const state = getWalkthroughNotificationState();
  if (timestamp === null) {
    delete state[taskId];
  } else {
    state[taskId] = timestamp;
  }
  setLocalStorage(WALKTHROUGH_NOTIFICATION_KEY, state);
}

export function getWalkthroughLastSeen(taskId: string): string | null {
  const state = getWalkthroughNotificationState();
  return state[taskId] ?? null;
}
