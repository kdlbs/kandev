import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useLinearEnabled } from "./use-linear-enabled";

const STORAGE_KEY = "kandev:linear:enabled:v1";

describe("useLinearEnabled", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });
  afterEach(() => {
    window.localStorage.clear();
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

  it("migrates a legacy per-workspace key into the new install-wide key on first read", async () => {
    // Single legacy entry so the migration outcome is deterministic — with
    // multiple, the "first one we encounter" depends on localStorage iteration
    // order, which the test shouldn't depend on.
    window.localStorage.setItem("kandev:linear:enabled:ws-1:v1", "false");
    const { result } = renderHook(() => useLinearEnabled());
    await waitFor(() => expect(result.current.loaded).toBe(true));

    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe("false");
    expect(window.localStorage.getItem("kandev:linear:enabled:ws-1:v1")).toBeNull();
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
