import { beforeEach, describe, expect, it, vi } from "vitest";
import { encodedBytes, _resetForTesting, MAX_ENTRY_BYTES } from "./buffer";
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
const TEST_SOURCE = "test";
const TEST_SCOPE = "default-user";

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

describe("browser logger scheduling", () => {
  it("keeps the collection window before scheduling browser-idle persistence", async () => {
    vi.useFakeTimers();
    let idleCallback: (() => void) | undefined;
    const requestIdle = vi.fn((callback: () => void, options: { timeout: number }) => {
      idleCallback = callback;
      return options.timeout;
    });
    const cancelIdle = vi.fn();
    vi.stubGlobal("requestIdleCallback", requestIdle);
    vi.stubGlobal("cancelIdleCallback", cancelIdle);

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "idle-after-window",
      });

      await vi.advanceTimersByTimeAsync(249);
      expect(requestIdle).not.toHaveBeenCalled();
      expect(storeMocks.append).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(1);
      expect(requestIdle).toHaveBeenCalledTimes(1);
      expect(requestIdle.mock.calls[0]?.[1]).toEqual({ timeout: 750 });
      expect(storeMocks.append).not.toHaveBeenCalled();

      idleCallback?.();
      await vi.waitFor(() => expect(storeMocks.append).toHaveBeenCalledTimes(1));
    } finally {
      vi.unstubAllGlobals();
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });

  it("uses the bounded deadline when browser idle time does not arrive", async () => {
    vi.useFakeTimers();
    const requestIdle = vi.fn(() => 7);
    const cancelIdle = vi.fn();
    vi.stubGlobal("requestIdleCallback", requestIdle);
    vi.stubGlobal("cancelIdleCallback", cancelIdle);

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "idle-deadline",
      });

      await vi.advanceTimersByTimeAsync(250);
      expect(storeMocks.append).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(749);
      expect(storeMocks.append).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(1);
      await vi.waitFor(() => expect(storeMocks.append).toHaveBeenCalledTimes(1));
      expect(cancelIdle).toHaveBeenCalledWith(7);
    } finally {
      vi.unstubAllGlobals();
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });

  it("bypasses a pending browser-idle wait when snapshotting", async () => {
    vi.useFakeTimers();
    let idleCallback: (() => void) | undefined;
    const requestIdle = vi.fn((callback: () => void) => {
      idleCallback = callback;
      return 7;
    });
    const cancelIdle = vi.fn();
    vi.stubGlobal("requestIdleCallback", requestIdle);
    vi.stubGlobal("cancelIdleCallback", cancelIdle);

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "snapshot-idle-bypass",
      });

      await vi.advanceTimersByTimeAsync(250);
      expect(requestIdle).toHaveBeenCalledTimes(1);
      expect(storeMocks.append).not.toHaveBeenCalled();

      await snapshotBrowserLogs(TEST_SCOPE);
      expect(storeMocks.append).toHaveBeenCalledTimes(1);
      expect(cancelIdle).toHaveBeenCalledWith(7);

      idleCallback?.();
      await vi.advanceTimersByTimeAsync(750);
      expect(storeMocks.append).toHaveBeenCalledTimes(1);
    } finally {
      vi.unstubAllGlobals();
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });
});

describe("browser logger collection", () => {
  it("coalesces a log burst in one 250 ms window before one bounded append", async () => {
    vi.useFakeTimers();

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "first",
      });
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "second",
      });

      expect(storeMocks.append).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(249);
      expect(storeMocks.append).not.toHaveBeenCalled();

      await vi.advanceTimersByTimeAsync(1);
      await vi.waitFor(() => expect(storeMocks.append).toHaveBeenCalledTimes(1));
      expect(storeMocks.append).toHaveBeenCalledWith(
        expect.arrayContaining([
          expect.objectContaining({ entry: expect.objectContaining({ message: "first" }) }),
          expect.objectContaining({ entry: expect.objectContaining({ message: "second" }) }),
        ]),
      );
    } finally {
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });

  it("passes one prepared entry and canonical byte count to persistence", async () => {
    vi.useFakeTimers();

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "prepared once",
        args: [{ detail: "value" }],
      });

      await vi.advanceTimersByTimeAsync(250);
      await vi.waitFor(() => expect(storeMocks.append).toHaveBeenCalledTimes(1));

      const batch = storeMocks.append.mock.calls[0]?.[0];
      expect(batch).toEqual([
        expect.objectContaining({
          entry: expect.objectContaining({ message: "prepared once" }),
          bytes: expect.any(Number),
        }),
      ]);
      expect(batch[0].bytes).toBe(encodedBytes(batch[0].entry));
    } finally {
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });
});

describe("browser logger runtime", () => {
  it("flushes a pending collection window before snapshotting", async () => {
    vi.useFakeTimers();

    try {
      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
        message: "flush before snapshot",
      });

      await vi.advanceTimersByTimeAsync(100);
      expect(storeMocks.append).not.toHaveBeenCalled();

      await snapshotBrowserLogs(TEST_SCOPE);
      expect(storeMocks.append).toHaveBeenCalledTimes(1);

      await vi.advanceTimersByTimeAsync(150);
      expect(storeMocks.append).toHaveBeenCalledTimes(1);
    } finally {
      vi.runOnlyPendingTimers();
      vi.useRealTimers();
    }
  });

  it("does not stage entries rejected by the bounded memory buffer", async () => {
    stageLogEntry({
      timestamp: new Date().toISOString(),
      level: "error",
      source: "window.error",
      message: "x".repeat(MAX_ENTRY_BYTES + 1),
    });

    await snapshotBrowserLogs(TEST_SCOPE);

    expect(storeMocks.append).not.toHaveBeenCalled();
  });

  it("falls back to memory when a persistence drain is rejected", async () => {
    storeMocks.append.mockRejectedValueOnce(new Error("IndexedDB upgrade blocked"));
    stageLogEntry({
      timestamp: new Date().toISOString(),
      level: "error",
      source: TEST_SOURCE,
      message: "kept in memory",
    });

    await expect(snapshotBrowserLogs(TEST_SCOPE)).resolves.toHaveLength(1);
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
        source: TEST_SOURCE,
        message: "first",
      });
      const snapshot = snapshotBrowserLogs(TEST_SCOPE);
      await vi.waitFor(() => expect(storeMocks.append).toHaveBeenCalledTimes(1));

      stageLogEntry({
        timestamp: new Date().toISOString(),
        level: "info",
        source: TEST_SOURCE,
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
