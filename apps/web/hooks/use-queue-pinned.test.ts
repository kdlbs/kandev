import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useQueuePinned, QUEUE_PIN_SYNC_EVENT, queuePinStorageKey } from "./use-queue-pinned";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";

const SESSION_A = "sess-a";
const SESSION_B = "sess-b";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

describe("useQueuePinned", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults to false when no pin entry exists for the session", () => {
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(false);
  });

  it('reads the literal string "true" only', () => {
    window.localStorage.setItem(queuePinStorageKey(SESSION_A), "true");
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(true);

    window.localStorage.setItem(queuePinStorageKey(SESSION_A), "1");
    const { result: off } = renderHook(() => useQueuePinned(SESSION_A));
    expect(off.current.value).toBe(false);
  });

  it("keeps pins isolated per session", () => {
    window.localStorage.setItem(queuePinStorageKey(SESSION_A), "true");
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(true);

    const { result: other } = renderHook(() => useQueuePinned(SESSION_B));
    expect(other.current.value).toBe(false);
  });

  it("setValue persists to the session-scoped key and updates state", async () => {
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(false);

    act(() => result.current.setValue(true));

    await waitFor(() => expect(result.current.value).toBe(true));
    expect(window.localStorage.getItem(queuePinStorageKey(SESSION_A))).toBe("true");

    act(() => result.current.setValue(false));
    await waitFor(() => expect(result.current.value).toBe(false));
    expect(window.localStorage.getItem(queuePinStorageKey(SESSION_A))).toBe("false");
  });

  it("propagates pin changes dispatched via the sync event", async () => {
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(false);

    act(() => {
      window.localStorage.setItem(queuePinStorageKey(SESSION_A), "true");
      window.dispatchEvent(new Event(QUEUE_PIN_SYNC_EVENT));
    });

    await waitFor(() => expect(result.current.value).toBe(true));
  });

  it("ignores sync events for other storage keys", () => {
    window.localStorage.setItem("kandev:unrelated:key:v1", "true");
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(false);

    act(() => {
      window.dispatchEvent(new Event("kandev:unrelated:changed"));
    });
    expect(result.current.value).toBe(false);
  });

  it("degrades to false for a null session without touching storage", () => {
    const getItem = vi.spyOn(localStorageMock, "getItem");
    const { result } = renderHook(() => useQueuePinned(null));
    expect(result.current.value).toBe(false);
    expect(getItem).not.toHaveBeenCalled();
  });

  it("setValue is a no-op for a null session", () => {
    const setItem = vi.spyOn(localStorageMock, "setItem");
    setItem.mockClear();
    const { result } = renderHook(() => useQueuePinned(null));
    act(() => result.current.setValue(true));
    expect(setItem).not.toHaveBeenCalled();
  });

  it("degrades to false when localStorage.getItem throws", () => {
    const original = localStorageMock.getItem;
    localStorageMock.getItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      const { result } = renderHook(() => useQueuePinned(SESSION_A));
      expect(result.current.value).toBe(false);
    } finally {
      localStorageMock.getItem = original;
    }
  });

  it("throws instead of reporting a successful save when localStorage.setItem fails", () => {
    const original = localStorageMock.setItem;
    localStorageMock.setItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      const { result } = renderHook(() => useQueuePinned(SESSION_A));
      expect(() => result.current.setValue(true)).toThrow();
      expect(result.current.value).toBe(false);
    } finally {
      localStorageMock.setItem = original;
    }
  });
});

describe("useQueuePinned toggle", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });

  it("flips the persisted value", async () => {
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    expect(result.current.value).toBe(false);

    act(() => result.current.toggle());
    await waitFor(() => expect(result.current.value).toBe(true));
    expect(window.localStorage.getItem(queuePinStorageKey(SESSION_A))).toBe("true");

    act(() => result.current.toggle());
    await waitFor(() => expect(result.current.value).toBe(false));
  });

  it("is a no-op for a null session", () => {
    const setItem = vi.spyOn(localStorageMock, "setItem");
    setItem.mockClear();
    const { result } = renderHook(() => useQueuePinned(null));
    act(() => result.current.toggle());
    expect(setItem).not.toHaveBeenCalled();
  });

  it("keeps the current-view pin active when persistence fails", async () => {
    const original = localStorageMock.setItem;
    const { result } = renderHook(() => useQueuePinned(SESSION_A));
    localStorageMock.setItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      expect(() => act(() => result.current.toggle())).not.toThrow();
      // The in-memory fallback flips the current view even though nothing
      // was persisted.
      expect(result.current.value).toBe(true);
      expect(window.localStorage.getItem(queuePinStorageKey(SESSION_A))).toBeNull();
    } finally {
      localStorageMock.setItem = original;
    }

    // A later successful write clears the volatile fallback and persists.
    act(() => result.current.toggle());
    await waitFor(() => expect(result.current.value).toBe(false));
    expect(window.localStorage.getItem(queuePinStorageKey(SESSION_A))).toBe("false");
  });
});
