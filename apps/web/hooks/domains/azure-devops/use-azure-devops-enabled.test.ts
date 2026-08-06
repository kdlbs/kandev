import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useAzureDevOpsEnabled } from "./use-azure-devops-enabled";

const STORAGE_KEY = "kandev:azure-devops:enabled:v1";

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

describe("useAzureDevOpsEnabled", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults to enabled=true when no localStorage entry exists", async () => {
    const { result } = renderHook(() => useAzureDevOpsEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);
  });

  it('reads enabled=false when stored as the literal string "false"', async () => {
    window.localStorage.setItem(STORAGE_KEY, "false");
    const { result } = renderHook(() => useAzureDevOpsEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);
  });

  it("setEnabled persists to localStorage and updates state", async () => {
    const { result } = renderHook(() => useAzureDevOpsEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    act(() => result.current.setEnabled(false));

    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
  });

  it("propagates updates dispatched via the kandev:azure-devops:enabled-changed event", async () => {
    const { result } = renderHook(() => useAzureDevOpsEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "false");
      window.dispatchEvent(new Event("kandev:azure-devops:enabled-changed"));
    });

    await waitFor(() => expect(result.current.enabled).toBe(false));
  });
});
