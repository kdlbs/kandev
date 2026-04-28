import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import type { JiraConfig } from "@/lib/types/jira";

const getJiraConfigMock = vi.fn<[string], Promise<JiraConfig | null>>();

vi.mock("@/lib/api/domains/jira-api", () => ({
  getJiraConfig: (workspaceId: string) => getJiraConfigMock(workspaceId),
}));

import { useJiraAvailable } from "./use-jira-availability";

function makeConfig(overrides: Partial<JiraConfig>): JiraConfig {
  return {
    workspaceId: "ws-1",
    siteUrl: "https://example.atlassian.net",
    email: "u@example.com",
    authMethod: "api_token",
    defaultProjectKey: "PROJ",
    hasSecret: true,
    lastOk: true,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("useJiraAvailable", () => {
  beforeEach(() => {
    window.localStorage.clear();
    getJiraConfigMock.mockReset();
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  it("returns false without a workspace id", () => {
    const { result } = renderHook(() => useJiraAvailable(undefined));
    expect(result.current).toBe(false);
    expect(getJiraConfigMock).not.toHaveBeenCalled();
  });

  it("returns true when enabled, configured, and auth is healthy", async () => {
    getJiraConfigMock.mockResolvedValue(makeConfig({ hasSecret: true, lastOk: true }));
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("returns false when the workspace toggle is disabled and never probes", async () => {
    window.localStorage.setItem("kandev:jira:enabled:ws-1:v1", "false");
    getJiraConfigMock.mockResolvedValue(makeConfig({ hasSecret: true, lastOk: true }));
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    // Disabled toggle should short-circuit the auth probe entirely. The
    // `loaded` guard inside the hook keeps the fetch from racing on first render.
    await waitFor(() => expect(result.current).toBe(false));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(getJiraConfigMock).not.toHaveBeenCalled();
    expect(result.current).toBe(false);
  });

  it("clears stale auth when the workspace switches", async () => {
    getJiraConfigMock.mockImplementation(async (id: string) =>
      id === "ws-1"
        ? makeConfig({ workspaceId: "ws-1", hasSecret: true, lastOk: true })
        : makeConfig({ workspaceId: "ws-2", hasSecret: false, lastOk: false }),
    );
    const { result, rerender } = renderHook(
      ({ id }: { id: string | undefined }) => useJiraAvailable(id),
      { initialProps: { id: "ws-1" as string | undefined } },
    );
    await waitFor(() => expect(result.current).toBe(true));
    rerender({ id: "ws-2" });
    // The previous workspace's `true` must not leak into the new one even
    // for the brief window before the new probe lands.
    expect(result.current).toBe(false);
    await waitFor(() => expect(result.current).toBe(false));
  });

  it("returns false when no secret is configured", async () => {
    getJiraConfigMock.mockResolvedValue(makeConfig({ hasSecret: false, lastOk: true }));
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    await waitFor(() => expect(getJiraConfigMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the most recent auth probe failed", async () => {
    getJiraConfigMock.mockResolvedValue(
      makeConfig({ hasSecret: true, lastOk: false, lastError: "401 Unauthorized" }),
    );
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    await waitFor(() => expect(getJiraConfigMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("returns false when the config request rejects", async () => {
    getJiraConfigMock.mockRejectedValue(new Error("network down"));
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    await waitFor(() => expect(getJiraConfigMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("does not flicker between poll ticks while auth stays healthy", async () => {
    vi.useFakeTimers();
    try {
      getJiraConfigMock.mockResolvedValue(makeConfig({ hasSecret: true, lastOk: true }));
      const seen: boolean[] = [];
      const { result } = renderHook(() => {
        const v = useJiraAvailable("ws-1");
        seen.push(v);
        return v;
      });
      // Wait for the first probe to resolve and flip the value to true.
      await vi.waitFor(() => expect(result.current).toBe(true));
      const beforeTick = [...seen];
      // Advance past one 90s poll. If the hook reset auth at the start of
      // every tick, we'd observe a false in `seen` between this tick and
      // the next probe response.
      await vi.advanceTimersByTimeAsync(95_000);
      expect(result.current).toBe(true);
      const newRenders = seen.slice(beforeTick.length);
      expect(newRenders).not.toContain(false);
    } finally {
      vi.useRealTimers();
    }
  });

  it("returns false when no config exists yet (backend 204)", async () => {
    getJiraConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    await waitFor(() => expect(getJiraConfigMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });
});
