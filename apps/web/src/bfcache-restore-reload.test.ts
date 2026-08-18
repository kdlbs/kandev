import { afterEach, describe, expect, it, vi } from "vitest";
import { installBfcacheRestoreReload } from "./bfcache-restore-reload";

const RESTORE_PROBE_KEY = "kandev.bfcacheRestoreProbe";

function createMemoryStorage() {
  const values = new Map<string, string>();
  return {
    storage: {
      setItem: vi.fn((key: string, value: string) => values.set(key, value)),
    },
    value: () => values.get(RESTORE_PROBE_KEY) ?? null,
  };
}

function createHarness({
  isDebug = () => false,
  storage = createMemoryStorage().storage,
  navigationType,
  injectNavigationType = true,
}: {
  isDebug?: () => boolean;
  storage?: Pick<Storage, "setItem">;
  navigationType?: string;
  injectNavigationType?: boolean;
} = {}) {
  const target = new EventTarget();
  const reload = vi.fn();
  const getNavigationType = vi.fn(() => navigationType);
  const cleanup = installBfcacheRestoreReload({
    target,
    reload,
    isDebug,
    getStorage: () => storage,
    ...(injectNavigationType ? { getNavigationType } : {}),
  });

  return {
    target,
    reload,
    storage,
    getNavigationType,
    cleanup,
    dispatch(persisted: boolean) {
      const event = new Event("pageshow");
      Object.defineProperty(event, "persisted", { value: persisted });
      target.dispatchEvent(event);
      return event;
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("installBfcacheRestoreReload", () => {
  it("reloads when the page is restored from a frozen snapshot (persisted=true)", () => {
    const harness = createHarness();

    harness.dispatch(true);

    expect(harness.reload).toHaveBeenCalledOnce();
  });

  it("does not reload on a cold back/forward traversal (persisted=false)", () => {
    // Regression for the round-1 finding: a history traversal not served
    // from bfcache (e.g. an open WebSocket made the no-store page ineligible)
    // loads fresh and must not be reloaded a second time, even when the
    // navigation entry reports type `back_forward`. The injected
    // navigation-type reader returns `back_forward` here, so reintroducing
    // the navigation-type reload fallback would fail this test.
    const harness = createHarness({ navigationType: "back_forward" });

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload on a cold back/forward traversal via the default reader (persisted=false)", () => {
    // Companion to the injected-seam case: a reintroduced fallback that calls
    // the module's DEFAULT readNavigationType (reading global performance)
    // must also fail. Stub global performance to report back_forward, install
    // without the injected reader, and assert no reload.
    vi.stubGlobal("performance", {
      // happy-dom's Event constructor calls performance.now(); keep it intact
      // while exposing the back_forward navigation entry.
      now: () => 0,
      getEntriesByType: (type: string) => (type === "navigation" ? [{ type: "back_forward" }] : []),
    });
    const harness = createHarness({ injectNavigationType: false });

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload on a fresh load (persisted=false)", () => {
    // navigationType: "navigate" distinguishes this from the manual-refresh
    // case below, so a reintroduced fallback keyed on either type fails.
    const harness = createHarness({ navigationType: "navigate" });

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload on a manual refresh (persisted=false)", () => {
    const harness = createHarness({ navigationType: "reload" });

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("does not reload when the event carries no persisted flag", () => {
    const harness = createHarness();

    harness.target.dispatchEvent(new Event("pageshow"));

    expect(harness.reload).not.toHaveBeenCalled();
  });

  it("uninstall removes the listener", () => {
    const harness = createHarness();

    harness.cleanup();
    harness.dispatch(true);

    expect(harness.reload).not.toHaveBeenCalled();
  });
});

describe("installBfcacheRestoreReload debug probe", () => {
  it("records restore evidence before reloading in debug builds", () => {
    const memory = createMemoryStorage();
    const harness = createHarness({
      isDebug: () => true,
      storage: memory.storage,
      navigationType: "back_forward",
    });

    harness.dispatch(true);

    expect(harness.reload).toHaveBeenCalledOnce();
    expect(memory.value()).not.toBeNull();
    expect(JSON.parse(memory.value()!)).toMatchObject({
      persisted: true,
      navigationType: "back_forward",
    });
    expect(Number.isFinite(JSON.parse(memory.value()!).at)).toBe(true);
  });

  it("records a persisted=false duplicate candidate without reloading", () => {
    // The merge gate must be able to distinguish "no pageshow at all" from
    // "pageshow with persisted=false": a duplicate that restores stale heap
    // state while reporting persisted=false (a cold back_forward traversal)
    // is recorded for diagnosis but never reloaded.
    const memory = createMemoryStorage();
    const harness = createHarness({
      isDebug: () => true,
      storage: memory.storage,
      navigationType: "back_forward",
    });

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
    expect(memory.value()).not.toBeNull();
    expect(JSON.parse(memory.value()!)).toMatchObject({
      persisted: false,
      navigationType: "back_forward",
    });
  });

  it("does not record non-candidates even in debug builds", () => {
    const memory = createMemoryStorage();
    const harness = createHarness({
      isDebug: () => true,
      storage: memory.storage,
      navigationType: "navigate",
    });

    harness.dispatch(false);

    expect(harness.reload).not.toHaveBeenCalled();
    expect(memory.value()).toBeNull();
  });

  it("does not record evidence when debug is disabled", () => {
    const memory = createMemoryStorage();
    const harness = createHarness({ isDebug: () => false, storage: memory.storage });

    harness.dispatch(true);

    expect(harness.reload).toHaveBeenCalledOnce();
    expect(memory.value()).toBeNull();
  });

  it("a failing probe never blocks the reload", () => {
    const harness = createHarness({
      isDebug: () => true,
      storage: {
        setItem: vi.fn(() => {
          throw new Error("storage unavailable");
        }),
      },
    });

    expect(() => harness.dispatch(true)).not.toThrow();
    expect(harness.reload).toHaveBeenCalledOnce();
  });

  it("a throwing debug predicate never blocks the reload", () => {
    const harness = createHarness({
      isDebug: () => {
        throw new Error("debug flag unavailable");
      },
    });

    expect(() => harness.dispatch(true)).not.toThrow();
    expect(harness.reload).toHaveBeenCalledOnce();
  });
});
