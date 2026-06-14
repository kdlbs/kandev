import { beforeEach, describe, expect, it } from "vitest";
import {
  cleanupTaskStorage,
  clearGlobalSidebarWidth,
  getGlobalSidebarWidth,
  markPRClosedBannerDismissed,
  markPRMergedBannerDismissed,
  restoreAttachmentPreview,
  setGlobalSidebarWidth,
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

describe("global sidebar width storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns null when no width has been stored", () => {
    expect(getGlobalSidebarWidth()).toBeNull();
  });

  it("persists a width globally and reads it back (rounded)", () => {
    setGlobalSidebarWidth(412.6);
    expect(getGlobalSidebarWidth()).toBe(413);
  });

  it("ignores non-positive or non-finite widths", () => {
    setGlobalSidebarWidth(0);
    setGlobalSidebarWidth(-100);
    setGlobalSidebarWidth(Number.NaN);
    expect(getGlobalSidebarWidth()).toBeNull();
  });

  it("clears the stored width", () => {
    setGlobalSidebarWidth(320);
    clearGlobalSidebarWidth();
    expect(getGlobalSidebarWidth()).toBeNull();
  });

  it("is NOT removed by cleanupTaskStorage (it is global, not task-scoped)", () => {
    setGlobalSidebarWidth(320);
    cleanupTaskStorage("task-a", []);
    expect(getGlobalSidebarWidth()).toBe(320);
  });
});

describe("chat draft attachment storage", () => {
  it("normalizes invalid restored image delivery modes to prompt", () => {
    const restored = restoreAttachmentPreview({
      id: "att-1",
      data: "abc",
      mimeType: "image/png",
      fileName: "shot.png",
      size: 3,
      isImage: true,
      deliveryMode: "inline" as "prompt",
    });

    expect(restored.deliveryMode).toBe("prompt");
    expect(restored.preview).toBe("data:image/png;base64,abc");
  });

  it("normalizes invalid restored file delivery modes to path", () => {
    const restored = restoreAttachmentPreview({
      id: "att-2",
      data: "abc",
      mimeType: "application/pdf",
      fileName: "doc.pdf",
      size: 3,
      isImage: false,
      deliveryMode: "inline" as "path",
    });

    expect(restored.deliveryMode).toBe("path");
  });
});
