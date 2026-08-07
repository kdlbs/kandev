import { beforeEach, describe, expect, it, vi } from "vitest";
import { _resetForTesting, MAX_ENTRY_BYTES } from "./buffer";
import { _resetRuntimeForTesting, snapshotBrowserLogs, stageLogEntry } from "./runtime";

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
});
