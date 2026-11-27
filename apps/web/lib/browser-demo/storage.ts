import { DEMO_STORAGE_KEY } from "./scenario";

export function readDemoSnapshot(): string | undefined {
  try {
    return window.sessionStorage.getItem(DEMO_STORAGE_KEY) ?? undefined;
  } catch {
    return undefined;
  }
}

export function saveDemoSnapshot(snapshot: string) {
  try {
    window.sessionStorage.setItem(DEMO_STORAGE_KEY, snapshot);
  } catch {
    // The current tab can keep running without persistent storage.
  }
}

export function dismissDemoOnboarding() {
  try {
    window.localStorage.setItem("kandev.onboarding.completed", "true");
  } catch {
    // Storage is optional for demo boot.
  }
}
