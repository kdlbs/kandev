import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useLinearEnabled } from "./use-linear-enabled";
import { resetIntegrationEnabledMigrations } from "../integrations/use-integration-enabled";

const STORAGE_KEY = "kandev:linear:enabled:v1";

// Provide a simple in-memory localStorage mock so the tests are not sensitive
// to how the test runner exposes window.localStorage in happy-dom.
function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => {
      store.set(key, value);
    },
    removeItem: (key: string) => {
      store.delete(key);
    },
    clear: () => {
      store.clear();
    },
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

describe("useLinearEnabled", () => {
  beforeEach(() => {
    localStorageMock.clear();
    resetIntegrationEnabledMigrations();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults to enabled=true when no localStorage entry exists", async () => {
    const { result } = renderHook(() => useLinearEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);
  });

  it('reads enabled=false when stored as the literal string "false"', async () => {
    window.localStorage.setItem(STORAGE_KEY, "false");
    const { result } = renderHook(() => useLinearEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);
  });

  it.each(["true", "1", "yes", "legacy"])(
    'treats persisted value %p as enabled — only the literal "false" disables',
    async (storedValue) => {
      window.localStorage.setItem(STORAGE_KEY, storedValue);
      const { result } = renderHook(() => useLinearEnabled());
      await waitFor(() => expect(result.current.loaded).toBe(true));
      expect(result.current.enabled).toBe(true);
    },
  );

  it("setEnabled persists to localStorage and updates state", async () => {
    const { result } = renderHook(() => useLinearEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    act(() => result.current.setEnabled(false));

    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
  });

  it("restores a legacy per-workspace key to that workspace's own key", async () => {
    // The pre-scoping keys were `kandev:linear:enabled:<workspaceId>:v1`, one per
    // workspace, so each is restored to the workspace it came from rather than
    // folded into a single install-wide value.
    window.localStorage.setItem("kandev:linear:enabled:ws-123:v1", "false");

    const { result } = renderHook(() => useLinearEnabled("ws-123"));

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(`${STORAGE_KEY}:ws-123`)).toBe("false");
    expect(window.localStorage.getItem("kandev:linear:enabled:ws-123:v1")).toBeNull();
    // Another workspace is untouched by that migration.
    expect(renderHook(() => useLinearEnabled("ws-other")).result.current.enabled).toBe(true);
  });

  it("propagates updates dispatched via the kandev:linear:enabled-changed event", async () => {
    const { result } = renderHook(() => useLinearEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "false");
      window.dispatchEvent(new Event("kandev:linear:enabled-changed"));
    });

    await waitFor(() => expect(result.current.enabled).toBe(false));
  });
});
