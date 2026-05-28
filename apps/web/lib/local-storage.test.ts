import { beforeEach, describe, expect, it } from "vitest";
import {
  cleanupTaskStorage,
  markPRMergedBannerDismissed,
  wasPRMergedBannerDismissed,
} from "./local-storage";

const DISMISSED_KEY_PREFIX = "kandev.pr-merged-banner-dismissed.";

describe("PR merged banner dismissal storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("returns false when no dismissal has been recorded", () => {
    expect(wasPRMergedBannerDismissed("task-a")).toBe(false);
  });

  it("persists dismissal per task and reads it back", () => {
    markPRMergedBannerDismissed("task-a");

    expect(wasPRMergedBannerDismissed("task-a")).toBe(true);
    expect(wasPRMergedBannerDismissed("task-b")).toBe(false);
    expect(window.sessionStorage.getItem(`${DISMISSED_KEY_PREFIX}task-a`)).toBe("1");
  });

  it("clears the dismissal flag via cleanupTaskStorage", () => {
    markPRMergedBannerDismissed("task-a");
    markPRMergedBannerDismissed("task-b");

    cleanupTaskStorage("task-a", []);

    expect(wasPRMergedBannerDismissed("task-a")).toBe(false);
    expect(wasPRMergedBannerDismissed("task-b")).toBe(true);
  });
});
