import { beforeEach, describe, expect, it, vi } from "vitest";
import { handleBrowserLogCapture } from "./capture";
import { prepareLogEntry, type LogEntry } from "./buffer";

const captureMocks = vi.hoisted(() => ({
  upload: vi.fn(),
  begin: vi.fn(),
  browserInstallationID: vi.fn(),
  browserLogMetadata: vi.fn(),
}));

vi.mock("@/lib/api/domains/system-api", () => ({
  uploadFrontendBundleChunk: captureMocks.upload,
}));

vi.mock("./runtime", () => ({
  beginBrowserLogCapture: captureMocks.begin,
  browserInstallationID: captureMocks.browserInstallationID,
  browserLogMetadata: captureMocks.browserLogMetadata,
}));

beforeEach(() => {
  captureMocks.upload.mockReset().mockResolvedValue(undefined);
  captureMocks.begin.mockReset();
  captureMocks.browserInstallationID.mockReset().mockReturnValue("browser");
  captureMocks.browserLogMetadata.mockReset().mockReturnValue({
    storage_mode: "indexeddb",
    persistence_failures: 0,
  });
});

describe("frontend log capture", () => {
  it("uploads each page before it reads the next page", async () => {
    const entries: LogEntry[] = [
      {
        timestamp: new Date(1).toISOString(),
        level: "info",
        source: "console",
        message: "x".repeat(60 * 1024),
      },
      {
        timestamp: new Date(2).toISOString(),
        level: "info",
        source: "console",
        message: "y".repeat(60 * 1024),
      },
    ];
    const prepared = entries.map(prepareLogEntry);
    const events: string[] = [];
    let releaseSecondPage: (() => void) | undefined;
    const secondPage = new Promise<void>((resolve) => {
      releaseSecondPage = resolve;
    });
    let readCount = 0;
    let resolveFirstUpload: (() => void) | undefined;
    const firstUploadStarted = new Promise<void>((resolve) => {
      resolveFirstUpload = resolve;
    });
    captureMocks.upload.mockImplementation(async (_id, chunk) => {
      events.push(`upload-${chunk.chunk_index}`);
      if (chunk.chunk_index === 0) resolveFirstUpload?.();
    });
    const readPage = vi.fn(async (maxBytes: number) => {
      events.push(`read-${readCount}`);
      readCount += 1;
      if (readCount === 1) {
        expect(maxBytes).toBe(128 * 1024);
        return {
          entries: prepared.slice(0, 1),
          nextCursor: { timestamp_ms: 1, primary_key: 1 },
          done: false,
        };
      }
      await secondPage;
      return { entries: prepared.slice(1), nextCursor: null, done: true };
    });
    captureMocks.begin.mockResolvedValue({
      storageMode: "indexeddb",
      flushTimeout: false,
      memoryEntries: prepared,
      readPage,
    });

    const capture = handleBrowserLogCapture(
      {
        bundle_id: "bundle",
        capture_deadline: new Date(Date.now() + 60_000).toISOString(),
        capture_timeout_ms: 15_000,
        max_chunk_bytes: 1024 * 1024,
        max_browser_profiles: 4,
      },
      "identity",
    );

    await firstUploadStarted;
    expect(events).toEqual(["read-0", "upload-0"]);
    expect(readPage).toHaveBeenCalledTimes(1);
    const firstEntries = captureMocks.upload.mock.calls[0]?.[1].entries as LogEntry[];
    const firstBytes = firstEntries.reduce(
      (total, entry) => total + new TextEncoder().encode(JSON.stringify(entry)).byteLength,
      0,
    );
    expect(firstBytes).toBeLessThanOrEqual(128 * 1024);

    releaseSecondPage?.();
    await capture;
    expect(captureMocks.upload).toHaveBeenCalledTimes(2);
    expect(readPage.mock.calls[1]?.[0]).toBe(800 * 1024);
    expect(events).toEqual(["read-0", "upload-0", "read-1", "upload-1"]);
  });
});

describe("frontend log capture deadlines and fallbacks", () => {
  it("uses the relative budget despite wall-clock skew and stops later pages", async () => {
    vi.useFakeTimers();
    const performanceNow = vi.spyOn(performance, "now");
    performanceNow.mockReturnValue(100);
    const first = prepareLogEntry({
      timestamp: new Date(1).toISOString(),
      level: "info",
      source: "console",
      message: "first",
    });
    const readPage = vi.fn().mockResolvedValue({
      entries: [first],
      nextCursor: { timestamp_ms: 1, primary_key: 1 },
      done: false,
    });
    captureMocks.begin.mockResolvedValue({
      storageMode: "indexeddb",
      flushTimeout: false,
      memoryEntries: [first],
      readPage,
    });
    captureMocks.upload.mockImplementation(async () => {
      performanceNow.mockReturnValue(200);
    });

    try {
      const capture = handleBrowserLogCapture(
        {
          bundle_id: "bundle",
          capture_deadline: new Date(0).toISOString(),
          capture_timeout_ms: 50,
          max_chunk_bytes: 1024 * 1024,
          max_browser_profiles: 4,
        },
        "identity",
      );

      await capture;
      expect(captureMocks.upload).toHaveBeenCalledTimes(1);
      expect(readPage).toHaveBeenCalledTimes(1);
    } finally {
      performanceNow.mockRestore();
      vi.useRealTimers();
    }
  });
});

describe("active frontend log capture deadlines", () => {
  it("aborts an active upload when the monotonic budget expires", async () => {
    vi.useFakeTimers();
    const performanceNow = vi.spyOn(performance, "now");
    performanceNow.mockReturnValue(100);
    let signal: AbortSignal | undefined;
    const entry = prepareLogEntry({
      timestamp: new Date(1).toISOString(),
      level: "info",
      source: "console",
      message: "active",
    });
    captureMocks.begin.mockResolvedValue({
      storageMode: "memory",
      flushTimeout: false,
      memoryEntries: [entry],
      readPage: null,
    });
    captureMocks.upload.mockImplementation((_id, _chunk, options) => {
      signal = options?.init?.signal;
      return new Promise<void>((_resolve, reject) => {
        signal?.addEventListener("abort", () => {
          reject(new DOMException("aborted", "AbortError"));
        });
      });
    });

    try {
      const capture = handleBrowserLogCapture(
        {
          bundle_id: "bundle",
          capture_deadline: new Date(0).toISOString(),
          capture_timeout_ms: 50,
          max_chunk_bytes: 1024 * 1024,
          max_browser_profiles: 4,
        },
        "identity",
      );
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(50);
      await expect(capture).resolves.toBeUndefined();
      expect(signal?.aborted).toBe(true);
    } finally {
      performanceNow.mockRestore();
      vi.useRealTimers();
    }
  });
});

describe("frontend log capture read fallback", () => {
  it("falls back to the receipt memory snapshot when a page read fails", async () => {
    const entry = prepareLogEntry({
      timestamp: new Date(1).toISOString(),
      level: "info",
      source: "console",
      message: "fallback",
    });
    const readPage = vi.fn().mockRejectedValue(new Error("IndexedDB unavailable"));
    captureMocks.begin.mockResolvedValue({
      storageMode: "indexeddb",
      flushTimeout: false,
      memoryEntries: [entry],
      readPage,
    });

    await handleBrowserLogCapture(
      {
        bundle_id: "bundle",
        capture_deadline: new Date(Date.now() + 60_000).toISOString(),
        capture_timeout_ms: 15_000,
        max_chunk_bytes: 1024 * 1024,
        max_browser_profiles: 4,
      },
      "identity",
    );

    expect(readPage).toHaveBeenCalledTimes(1);
    expect(captureMocks.upload.mock.calls[0]?.[1]).toMatchObject({
      done: true,
      storage_mode: "memory",
      entries: [entry.entry],
    });
  });

  it("does not switch sources after the first page has uploaded", async () => {
    const entry = prepareLogEntry({
      timestamp: new Date(1).toISOString(),
      level: "info",
      source: "console",
      message: "first page",
    });
    const readPage = vi
      .fn()
      .mockResolvedValueOnce({
        entries: [entry],
        nextCursor: { timestamp_ms: 1, primary_key: 1 },
        done: false,
      })
      .mockRejectedValueOnce(new Error("IndexedDB read failed"));
    captureMocks.begin.mockResolvedValue({
      storageMode: "indexeddb",
      flushTimeout: false,
      memoryEntries: [entry],
      readPage,
    });

    await expect(
      handleBrowserLogCapture(
        {
          bundle_id: "bundle",
          capture_deadline: new Date(Date.now() + 60_000).toISOString(),
          capture_timeout_ms: 15_000,
          max_chunk_bytes: 1024 * 1024,
          max_browser_profiles: 4,
        },
        "identity",
      ),
    ).rejects.toThrow("IndexedDB read failed");

    expect(captureMocks.upload).toHaveBeenCalledTimes(1);
  });
});

describe("frontend log capture metadata", () => {
  it("marks a timed-out persistence flush in final capture metadata", async () => {
    const entry = prepareLogEntry({
      timestamp: new Date(1).toISOString(),
      level: "info",
      source: "console",
      message: "memory",
    });
    captureMocks.begin.mockResolvedValue({
      storageMode: "memory",
      flushTimeout: true,
      memoryEntries: [entry],
      readPage: null,
    });

    await handleBrowserLogCapture(
      {
        bundle_id: "bundle",
        capture_deadline: new Date(Date.now() + 60_000).toISOString(),
        capture_timeout_ms: 15_000,
        max_chunk_bytes: 1024 * 1024,
        max_browser_profiles: 4,
      },
      "identity",
    );

    expect(captureMocks.upload.mock.calls[0]?.[1]).toMatchObject({
      done: true,
      storage_mode: "memory",
      capture_metadata: { flush_timeout: true, storage_mode: "memory" },
    });
  });
});
