import { beforeEach, describe, expect, it, vi } from "vitest";
import { _resetForTesting, MAX_ENTRY_BYTES } from "./buffer";
import {
  _resetRuntimeForTesting,
  browserLogMetadata,
  snapshotBrowserLogs,
  stageLogEntry,
} from "./runtime";

const storeMocks = vi.hoisted(() => ({
  append: vi.fn(),
  snapshot: vi.fn(),
}));

vi.mock("./indexeddb-store", () => ({
  IndexedDBLogStore: class {
    append = storeMocks.append;
    snapshot = storeMocks.snapshot;
  },
}));

beforeEach(() => {
  _resetForTesting();
  _resetRuntimeForTesting();
  storeMocks.append.mockReset();
  storeMocks.snapshot.mockReset().mockResolvedValue([]);
});

describe("browser logger runtime", () => {
  it("does not stage entries rejected by the bounded memory buffer", async () => {
    stageLogEntry({
      timestamp: new Date().toISOString(),
      level: "error",
      source: "window.error",
      message: "x".repeat(MAX_ENTRY_BYTES + 1),
    });

    await snapshotBrowserLogs("default-user");

    expect(storeMocks.append).not.toHaveBeenCalled();
  });

  it("falls back to memory when a persistence drain is rejected", async () => {
    storeMocks.append.mockRejectedValueOnce(new Error("IndexedDB upgrade blocked"));
    stageLogEntry({
      timestamp: new Date().toISOString(),
      level: "error",
      source: "test",
      message: "kept in memory",
    });

    await expect(snapshotBrowserLogs("default-user")).resolves.toHaveLength(1);
    expect(browserLogMetadata()).toMatchObject({
      storage_mode: "memory",
      persistence_failures: 1,
    });
  });

  it("serializes runtime drains and makes snapshots join the in-flight drain", async () => {
    vi.useFakeTimers();
    let activeAppends = 0;
    let maximumActiveAppends = 0;
    let appendCalls = 0;
    let releaseFirstAppend: (() => void) | undefined;
    const firstAppend = new Promise<void>((resolve) => {
      releaseFirstAppend = resolve;
    });
    storeMocks.append.mockImplementation(async () => {
      appendCalls += 1;
      activeAppends += 1;
      maximumActiveAppends = Math.max(maximumActiveAppends, activeAppends);
      if (appendCalls === 1) await firstAppend;
      activeAppends -= 1;
    });

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: "test",
        message: "first",
      });
      const snapshot = snapshotBrowserLogs("default-user");
      await vi.waitFor(() => expect(storeMocks.append).toHaveBeenCalledTimes(1));

      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: "test",
        message: "second",
      });
      vi.runOnlyPendingTimers();
      await Promise.resolve();
      expect(storeMocks.append).toHaveBeenCalledTimes(1);
      releaseFirstAppend?.();

      await snapshot;
      expect(storeMocks.append).toHaveBeenCalledTimes(2);
      expect(maximumActiveAppends).toBe(1);
    } finally {
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });
});
