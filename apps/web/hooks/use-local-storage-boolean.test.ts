import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useLocalStorageBoolean } from "./use-local-storage-boolean";
import { makeLocalStorageMock } from "@/hooks/local-storage-mock.test-helpers";

const STORAGE_KEY = "kandev:generic:flag:v1";
const SYNC_EVENT = "kandev:generic:flag-changed";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

describe("useLocalStorageBoolean", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults to the caller's defaultValue when no entry exists", () => {
    const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT, true));
    expect(result.current.value).toBe(true);
  });

  it("defaults to false when no entry exists and defaultValue is omitted", () => {
    const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
    expect(result.current.value).toBe(false);
  });

  it('reads the literal string "true" only', () => {
    window.localStorage.setItem(STORAGE_KEY, "true");
    const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
    expect(result.current.value).toBe(true);

    window.localStorage.setItem(STORAGE_KEY, "1");
    const { result: off } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
    expect(off.current.value).toBe(false);
  });

  it("setValue persists to the caller's key and updates state", async () => {
    const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
    expect(result.current.value).toBe(false);

    act(() => result.current.setValue(true));

    await waitFor(() => expect(result.current.value).toBe(true));
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("true");
  });

  it("propagates updates dispatched via the caller's sync event", async () => {
    const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
    expect(result.current.value).toBe(false);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "true");
      window.dispatchEvent(new Event(SYNC_EVENT));
    });

    await waitFor(() => expect(result.current.value).toBe(true));
  });

  it("ignores other storage keys' values and does not cross-talk sync events", () => {
    const otherKey = "kandev:other:flag:v1";
    const otherEvent = "kandev:other:flag-changed";
    window.localStorage.setItem(otherKey, "true");
    const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
    expect(result.current.value).toBe(false);

    act(() => {
      window.dispatchEvent(new Event(otherEvent));
    });
    expect(result.current.value).toBe(false);
  });

  it("degrades to the default when localStorage.getItem throws", () => {
    const original = localStorageMock.getItem;
    localStorageMock.getItem = () => {
      throw new Error("quota exceeded");
    };
    try {
      const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
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
      const { result } = renderHook(() => useLocalStorageBoolean(STORAGE_KEY, SYNC_EVENT));
      expect(() => result.current.setValue(true)).toThrow();
      expect(result.current.value).toBe(false);
    } finally {
      localStorageMock.setItem = original;
    }
  });
});
