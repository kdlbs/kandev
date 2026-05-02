import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { useLinearEnabled } from "./use-linear-enabled";

const STORAGE_KEY = (workspaceId: string) => `kandev:linear:enabled:${workspaceId}:v1`;

describe("useLinearEnabled", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });
  afterEach(() => {
    window.localStorage.clear();
  });

  it("defaults to enabled=true when no localStorage entry exists", async () => {
    const { result } = renderHook(() => useLinearEnabled("ws-1"));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);
  });

  it('reads enabled=false when stored as the literal string "false"', async () => {
    window.localStorage.setItem(STORAGE_KEY("ws-1"), "false");
    const { result } = renderHook(() => useLinearEnabled("ws-1"));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);
  });

  it.each(["true", "1", "yes", "legacy"])(
    'treats persisted value %p as enabled — only the literal "false" disables',
    async (storedValue) => {
      window.localStorage.setItem(STORAGE_KEY("ws-1"), storedValue);
      const { result } = renderHook(() => useLinearEnabled("ws-1"));
      await waitFor(() => expect(result.current.loaded).toBe(true));
      expect(result.current.enabled).toBe(true);
    },
  );

  it("setEnabled persists to localStorage and updates state", async () => {
    const { result } = renderHook(() => useLinearEnabled("ws-1"));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    act(() => result.current.setEnabled(false));

    expect(result.current.enabled).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEY("ws-1"))).toBe("false");
  });

  it("ignores writes when no workspaceId is provided", async () => {
    const { result } = renderHook(() => useLinearEnabled(undefined));
    await waitFor(() => expect(result.current.loaded).toBe(true));

    act(() => result.current.setEnabled(false));

    // No workspace key means no persisted state and no in-memory flip — the
    // setter is a no-op rather than silently writing under a sentinel key.
    expect(result.current.enabled).toBe(true);
    expect(window.localStorage.length).toBe(0);
  });

  it("re-runs the read when workspaceId changes", async () => {
    window.localStorage.setItem(STORAGE_KEY("ws-1"), "false");
    window.localStorage.setItem(STORAGE_KEY("ws-2"), "true");

    const { result, rerender } = renderHook(({ id }) => useLinearEnabled(id), {
      initialProps: { id: "ws-1" as string | undefined },
    });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(false);

    rerender({ id: "ws-2" });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);
  });

  it("propagates updates dispatched via the kandev:linear:enabled-changed event", async () => {
    const { result } = renderHook(() => useLinearEnabled("ws-1"));
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.enabled).toBe(true);

    act(() => {
      window.localStorage.setItem(STORAGE_KEY("ws-1"), "false");
      window.dispatchEvent(new Event("kandev:linear:enabled-changed"));
    });

    await waitFor(() => expect(result.current.enabled).toBe(false));
  });
});
