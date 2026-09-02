import { beforeEach, describe, expect, it } from "vitest";
import {
  cleanupTaskStorage,
  clearGlobalSidebarWidth,
  getGlobalSidebarWidth,
  getEnvLayoutProfile,
  getManualRightWidth,
  getOpenFileTabs,
  getStoredAutoScrollEnabled,
  getStoredAutoScrollTop,
  markPRClosedBannerDismissed,
  markPRMergedBannerDismissed,
  markPRPanelOffered,
  restoreAttachmentPreview,
  removeEnvLayoutProfile,
  setGlobalSidebarWidth,
  setEnvLayoutProfile,
  setManualRightWidth,
  clearManualRightWidth,
  setOpenFileTabs,
  setStoredAutoScrollEnabled,
  setStoredAutoScrollTop,
  wasPRClosedBannerDismissed,
  wasPRMergedBannerDismissed,
  wasPRPanelOffered,
} from "./local-storage";
import {
  loadSessionFavorites,
  persistSessionFavorites,
} from "./state/slices/message-favorites/persistence";

describe("message favorites storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("clears a session's favorites via cleanupTaskStorage but leaves other sessions intact", () => {
    persistSessionFavorites("session-a", ["msg-1"]);
    persistSessionFavorites("session-b", ["msg-2"]);

    cleanupTaskStorage("task-a", ["session-a"]);

    expect(loadSessionFavorites("session-a")).toEqual([]);
    expect(loadSessionFavorites("session-b")).toEqual(["msg-2"]);
  });
});

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

describe("PR panel offered storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("persists the offered flag per session", () => {
    expect(wasPRPanelOffered("session-a")).toBe(false);

    markPRPanelOffered("session-a");

    expect(wasPRPanelOffered("session-a")).toBe(true);
    expect(wasPRPanelOffered("session-b")).toBe(false);
  });

  it("clears the offered flag via cleanupTaskStorage", () => {
    markPRPanelOffered("session-a");
    markPRPanelOffered("session-b");

    cleanupTaskStorage("task-a", ["session-a"]);

    expect(wasPRPanelOffered("session-a")).toBe(false);
    expect(wasPRPanelOffered("session-b")).toBe(true);
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

describe("manual right width storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("keeps a rounded width scoped to its environment", () => {
    setManualRightWidth("env-a", 421.6);

    expect(getManualRightWidth("env-a")).toBe(422);
    expect(getManualRightWidth("env-b")).toBeNull();
  });

  it("ignores invalid widths and can clear a preference", () => {
    setManualRightWidth("env-a", 0);
    setManualRightWidth("env-a", Number.NaN);
    expect(getManualRightWidth("env-a")).toBeNull();

    setManualRightWidth("env-a", 320);
    clearManualRightWidth("env-a");
    expect(getManualRightWidth("env-a")).toBeNull();
  });

  it("cleans only the deleted task environments while preserving other widths", () => {
    setManualRightWidth("env-a", 320);
    setManualRightWidth("env-b", 420);

    cleanupTaskStorage("task-a", [], ["env-a"]);

    expect(getManualRightWidth("env-a")).toBeNull();
    expect(getManualRightWidth("env-b")).toBe(420);
  });
});

describe("dockview layout profile storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("round-trips profile identity per environment", () => {
    setEnvLayoutProfile("env-a", { kind: "custom", id: "layout-copied-default" });

    expect(getEnvLayoutProfile("env-a")).toEqual({
      kind: "custom",
      id: "layout-copied-default",
    });
    expect(getEnvLayoutProfile("env-b")).toBeNull();
  });

  it("ignores malformed profile identity and supports explicit removal", () => {
    window.sessionStorage.setItem(
      "kandev.dockview.env-layout-profile-v1.env-a",
      JSON.stringify({ kind: "custom" }),
    );
    expect(getEnvLayoutProfile("env-a")).toBeNull();

    setEnvLayoutProfile("env-a", { kind: "built-in", id: "default" });
    removeEnvLayoutProfile("env-a");
    expect(getEnvLayoutProfile("env-a")).toBeNull();
  });

  it("cleans profile identity with the deleted environment", () => {
    setEnvLayoutProfile("env-a", { kind: "built-in", id: "default" });
    setEnvLayoutProfile("env-b", { kind: "custom", id: "other" });

    cleanupTaskStorage("task-a", [], ["env-a"]);

    expect(getEnvLayoutProfile("env-a")).toBeNull();
    expect(getEnvLayoutProfile("env-b")).toEqual({ kind: "custom", id: "other" });
  });
});

describe("task-scoped artifact notification storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("clears walkthrough last-seen state via cleanupTaskStorage", () => {
    window.localStorage.setItem(
      "kandev.walkthrough.lastSeenByTask",
      JSON.stringify({ "task-a": "2026-01-01T00:00:00Z", "task-b": "2026-01-02T00:00:00Z" }),
    );

    cleanupTaskStorage("task-a", []);

    expect(
      JSON.parse(window.localStorage.getItem("kandev.walkthrough.lastSeenByTask") ?? "{}"),
    ).toEqual({
      "task-b": "2026-01-02T00:00:00Z",
    });
  });
});

describe("open file tabs storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("round-trips the multi-repo repo subpath so a restored tab refetches under the right repo", () => {
    setOpenFileTabs("sess-1", [
      {
        path: "src/foo.md",
        name: "foo.md",
        repo: "enrichment-commons",
        markdownMode: "preview",
        pinned: true,
      },
    ]);

    const tabs = getOpenFileTabs("sess-1");
    expect(tabs).toHaveLength(1);
    expect(tabs[0]).toEqual({
      path: "src/foo.md",
      name: "foo.md",
      repo: "enrichment-commons",
      markdownMode: "preview",
      pinned: true,
    });
  });

  it("leaves repo undefined for single-repo tabs", () => {
    setOpenFileTabs("sess-1", [{ path: "src/foo.ts", name: "foo.ts", pinned: true }]);
    expect(getOpenFileTabs("sess-1")[0].repo).toBeUndefined();
  });

  it("migrates legacy Markdown preview flags to an explicit mode", () => {
    const previewPath = "preview.md";
    const sourcePath = "source.md";
    const missingPath = "missing.md";
    const pinned = true;

    window.sessionStorage.setItem(
      "kandev.openFiles.sess-legacy",
      JSON.stringify([
        { path: previewPath, name: previewPath, markdownPreview: true, pinned },
        { path: sourcePath, name: sourcePath, markdownPreview: false, pinned },
        { path: missingPath, name: missingPath, pinned },
      ]),
    );

    expect(getOpenFileTabs("sess-legacy")).toEqual([
      { path: previewPath, name: previewPath, markdownMode: "preview", pinned },
      { path: sourcePath, name: sourcePath, markdownMode: "source", pinned },
      { path: missingPath, name: missingPath, markdownMode: "source", pinned },
    ]);
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

describe("transcript auto-scroll storage", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("returns null for a session with no recorded preference", () => {
    expect(getStoredAutoScrollEnabled("session-a")).toBeNull();
  });

  it("persists the enabled preference per session and reads it back", () => {
    setStoredAutoScrollEnabled("session-a", false);

    expect(getStoredAutoScrollEnabled("session-a")).toBe(false);
    expect(getStoredAutoScrollEnabled("session-b")).toBeNull();
  });

  it("persists the last scrollTop per session and reads it back", () => {
    setStoredAutoScrollTop("session-a", 480);

    expect(getStoredAutoScrollTop("session-a")).toBe(480);
    expect(getStoredAutoScrollTop("session-b")).toBeNull();
  });

  it("clears both the enabled preference and scrollTop via cleanupTaskStorage", () => {
    setStoredAutoScrollEnabled("session-a", false);
    setStoredAutoScrollTop("session-a", 480);
    setStoredAutoScrollEnabled("session-b", false);
    setStoredAutoScrollTop("session-b", 200);

    cleanupTaskStorage("task-a", ["session-a"]);

    expect(getStoredAutoScrollEnabled("session-a")).toBeNull();
    expect(getStoredAutoScrollTop("session-a")).toBeNull();
    expect(getStoredAutoScrollEnabled("session-b")).toBe(false);
    expect(getStoredAutoScrollTop("session-b")).toBe(200);
  });
});
