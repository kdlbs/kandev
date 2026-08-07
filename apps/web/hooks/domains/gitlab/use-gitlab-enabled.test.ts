import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useGitLabEnabled } from "./use-gitlab-enabled";
import { makeLocalStorageMock } from "../integrations/local-storage-mock.test-helpers";

const STORAGE_KEY = "kandev:gitlab:enabled:v1";

const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);
Object.defineProperty(window, "localStorage", {
  value: localStorageMock,
  configurable: true,
});

describe("useGitLabEnabled", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });
  afterEach(() => {
    localStorageMock.clear();
  });

  it("defaults to enabled=true when no localStorage entry exists", async () => {
    const { result } = renderHook(() => useGitLabEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);
  });

  it('reads enabled=false when stored as the literal string "false"', async () => {
    window.localStorage.setItem(STORAGE_KEY, "false");
    const { result } = renderHook(() => useGitLabEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);
  });

  it("setEnabled persists to localStorage and updates state", async () => {
    const { result } = renderHook(() => useGitLabEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    act(() => result.current.setEnabled(false));

    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
  });

  it("propagates updates dispatched via the kandev:gitlab:enabled-changed event", async () => {
    const { result } = renderHook(() => useGitLabEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "false");
      window.dispatchEvent(new Event("kandev:gitlab:enabled-changed"));
    });

    await waitFor(() => expect(result.current.enabled).toBe(false));
  });

  it("propagates updates delivered via the native storage event (cross-tab)", async () => {
    const { result } = renderHook(() => useGitLabEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    act(() => {
      window.localStorage.setItem(STORAGE_KEY, "false");
      window.dispatchEvent(new StorageEvent("storage", { key: STORAGE_KEY, newValue: "false" }));
    });

    await waitFor(() => expect(result.current.enabled).toBe(false));
  });

  it("migrates a legacy per-workspace key onto the canonical storage key", async () => {
    window.localStorage.setItem("kandev:gitlab:enabled:ws-123", "false");

    const { result } = renderHook(() => useGitLabEnabled());

    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
    expect(window.localStorage.getItem("kandev:gitlab:enabled:ws-123")).toBeNull();
  });
});
