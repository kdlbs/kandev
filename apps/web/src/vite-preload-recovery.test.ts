import { describe, expect, it, vi } from "vitest";
import { installVitePreloadRecovery } from "./vite-preload-recovery";

const PRELOAD_RECOVERY_KEY = "kandev.preloadRecovery";
const NOW = 120_000;
const HREF = "https://kandev.test/settings";

type RecoveryStorage = Pick<Storage, "getItem" | "setItem">;

function createMemoryStorage(initialMarker?: string) {
  const values = new Map<string, string>();
  if (initialMarker !== undefined) {
    values.set(PRELOAD_RECOVERY_KEY, initialMarker);
  }

  return {
    storage: {
      getItem: vi.fn((key: string) => values.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => values.set(key, value)),
    } satisfies RecoveryStorage,
    marker: () => values.get(PRELOAD_RECOVERY_KEY) ?? null,
  };
}

function createHarness(getStorage: () => RecoveryStorage = () => createMemoryStorage().storage) {
  const target = new EventTarget();
  const reload = vi.fn();
  const cleanup = installVitePreloadRecovery({
    target,
    getStorage,
    now: () => NOW,
    getHref: () => HREF,
    reload,
  });

  return {
    target,
    reload,
    cleanup,
    dispatch() {
      const event = new Event("vite:preloadError", { cancelable: true });
      target.dispatchEvent(event);
      return event;
    },
  };
}

function defineRetryTests() {
  it("marks, prevents, and reloads the first preload failure", () => {
    const memory = createMemoryStorage();
    const harness = createHarness(() => memory.storage);

    const event = harness.dispatch();

    expect({
      marker: memory.marker(),
      defaultPrevented: event.defaultPrevented,
      reloadCalls: harness.reload.mock.calls.length,
    }).toEqual({
      marker: JSON.stringify({ attemptedAt: NOW, href: HREF }),
      defaultPrevented: true,
      reloadCalls: 1,
    });
  });

  it("lets a recent repeated failure reach the error boundary", () => {
    const marker = JSON.stringify({ attemptedAt: NOW - 59_999, href: "https://kandev.test/" });
    const memory = createMemoryStorage(marker);
    const harness = createHarness(() => memory.storage);

    const event = harness.dispatch();

    expect({
      marker: memory.marker(),
      reads: memory.storage.getItem.mock.calls.length,
      defaultPrevented: event.defaultPrevented,
      reloadCalls: harness.reload.mock.calls.length,
    }).toEqual({
      marker,
      reads: 1,
      defaultPrevented: false,
      reloadCalls: 0,
    });
  });

  it.each([
    ["expired", JSON.stringify({ attemptedAt: NOW - 60_000, href: HREF })],
    ["future-dated", JSON.stringify({ attemptedAt: NOW + 1, href: HREF })],
    ["corrupt", "{not-json"],
  ])("replaces the %s marker and retries once", (_description, initialMarker) => {
    const memory = createMemoryStorage(initialMarker);
    const harness = createHarness(() => memory.storage);

    const event = harness.dispatch();

    expect({
      marker: memory.marker(),
      defaultPrevented: event.defaultPrevented,
      reloadCalls: harness.reload.mock.calls.length,
    }).toEqual({
      marker: JSON.stringify({ attemptedAt: NOW, href: HREF }),
      defaultPrevented: true,
      reloadCalls: 1,
    });
  });
}

function defineStorageFailureTests() {
  it.each([
    [
      "unavailable",
      () => {
        throw new Error("sessionStorage unavailable");
      },
    ],
    [
      "unwritable",
      () =>
        ({
          getItem: vi.fn(() => null),
          setItem: vi.fn(() => {
            throw new Error("quota exceeded");
          }),
        }) satisfies RecoveryStorage,
    ],
    [
      "unverifiable",
      () => {
        let reads = 0;
        return {
          getItem: vi.fn(() => {
            reads += 1;
            return reads === 1 ? null : JSON.stringify({ attemptedAt: NOW - 1, href: HREF });
          }),
          setItem: vi.fn(),
        } satisfies RecoveryStorage;
      },
    ],
  ])("does not prevent or reload when storage is %s", (_description, getStorage) => {
    const trackedGetStorage = vi.fn(getStorage);
    const harness = createHarness(trackedGetStorage);

    const event = harness.dispatch();

    expect({
      storageAttempts: trackedGetStorage.mock.calls.length,
      defaultPrevented: event.defaultPrevented,
      reloadCalls: harness.reload.mock.calls.length,
    }).toEqual({
      storageAttempts: 1,
      defaultPrevented: false,
      reloadCalls: 0,
    });
  });
}

function defineCleanupTests() {
  it("removes the preload listener during cleanup", () => {
    const memory = createMemoryStorage();
    const target = new EventTarget();
    const removeEventListener = vi.spyOn(target, "removeEventListener");
    const cleanup = installVitePreloadRecovery({
      target,
      getStorage: () => memory.storage,
      now: () => NOW,
      getHref: () => HREF,
      reload: vi.fn(),
    });

    cleanup();
    target.dispatchEvent(new Event("vite:preloadError", { cancelable: true }));

    expect({
      removed: removeEventListener.mock.calls.length,
      marker: memory.marker(),
    }).toEqual({
      removed: 1,
      marker: null,
    });
  });
}

describe("installVitePreloadRecovery", () => {
  defineRetryTests();
  defineStorageFailureTests();
  defineCleanupTests();
});
