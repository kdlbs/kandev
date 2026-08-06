import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useHideDisabledIntegrationsInNav } from "./use-hide-disabled-integrations-in-nav";

const STORAGE_KEY = "kandev:integrations:hideDisabledInNav:v1";

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

describe("useHideDisabledIntegrationsInNav", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults to hideDisabled=false when no localStorage entry exists", () => {
    const { result } = renderHook(() => useHideDisabledIntegrationsInNav());
    expect(result.current.hideDisabled).toBe(false);
  });

  it('reads hideDisabled=true only when stored as the literal string "true"', () => {
    window.localStorage.setItem(STORAGE_KEY, "true");
    const { result } = renderHook(() => useHideDisabledIntegrationsInNav());
    expect(result.current.hideDisabled).toBe(true);
  });

  it.each(["false", "1", "yes", ""])(
    'treats persisted value %p as off — only the literal "true" enables it',
    (storedValue) => {
      window.localStorage.setItem(STORAGE_KEY, storedValue);
      const { result } = renderHook(() => useHideDisabledIntegrationsInNav());
      expect(result.current.hideDisabled).toBe(false);
    },
  );

  it("setHideDisabled persists to localStorage and updates state", async () => {
    const { result } = renderHook(() => useHideDisabledIntegrationsInNav());
    expect(result.current.hideDisabled).toBe(false);

    act(() => result.current.setHideDisabled(true));

    await waitFor(() => expect(result.current.hideDisabled).toBe(true));
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("true");
  });

  it("propagates updates dispatched via the kandev:integrations:hide-disabled-in-nav-changed event", async () => {
    const { result } = renderHook(() => useHideDisabledIntegrationsInNav());
    expect(result.current.hideDisabled).toBe(false);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "true");
      window.dispatchEvent(new Event("kandev:integrations:hide-disabled-in-nav-changed"));
    });

    await waitFor(() => expect(result.current.hideDisabled).toBe(true));
  });

  it("degrades to hideDisabled=false when localStorage.getItem throws", () => {
    const original = localStorageMock.getItem;
    localStorageMock.getItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      const { result } = renderHook(() => useHideDisabledIntegrationsInNav());
      expect(result.current.hideDisabled).toBe(false);
    } finally {
      localStorageMock.getItem = original;
    }
  });
});
