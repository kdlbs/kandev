import { beforeEach, describe, expect, it } from "vitest";
import {
  cleanupTaskStorage,
  markPRClosedBannerDismissed,
  markPRMergedBannerDismissed,
  wasPRClosedBannerDismissed,
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

describe("PR closed banner dismissal storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("returns false when no dismissal has been recorded", () => {
    expect(wasPRClosedBannerDismissed("task-a")).toBe(false);
  });

  it("persists dismissal per task and reads it back", () => {
    markPRClosedBannerDismissed("task-a");

    expect(wasPRClosedBannerDismissed("task-a")).toBe(true);
    expect(wasPRClosedBannerDismissed("task-b")).toBe(false);
  });

  it("is independent from the merged banner dismissal flag", () => {
    markPRMergedBannerDismissed("task-a");

    expect(wasPRClosedBannerDismissed("task-a")).toBe(false);
  });

  it("clears the dismissal flag via cleanupTaskStorage", () => {
    markPRClosedBannerDismissed("task-a");
    markPRClosedBannerDismissed("task-b");

    cleanupTaskStorage("task-a", []);

    expect(wasPRClosedBannerDismissed("task-a")).toBe(false);
    expect(wasPRClosedBannerDismissed("task-b")).toBe(true);
  });
});
