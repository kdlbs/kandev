import { beforeEach, describe, expect, it } from "vitest";
import {
  cleanupTaskStorage,
  markPRMergedBannerDismissed,
  wasPRMergedBannerDismissed,
} from "./local-storage";

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
  });

  it("clears the dismissal flag via cleanupTaskStorage", () => {
    markPRMergedBannerDismissed("task-a");
    markPRMergedBannerDismissed("task-b");

    cleanupTaskStorage("task-a", []);

    expect(wasPRMergedBannerDismissed("task-a")).toBe(false);
    expect(wasPRMergedBannerDismissed("task-b")).toBe(true);
  });
});
