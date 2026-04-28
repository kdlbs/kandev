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

  it("returns false when the workspace toggle is disabled", async () => {
    window.localStorage.setItem("kandev:jira:enabled:ws-1:v1", "false");
    getJiraConfigMock.mockResolvedValue(makeConfig({ hasSecret: true, lastOk: true }));
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    // Even with a healthy backend config, the disabled toggle keeps the UI hidden.
    await waitFor(() => expect(result.current).toBe(false));
    // Give the effect a microtask to confirm it stays false.
    await Promise.resolve();
    expect(result.current).toBe(false);
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

  it("returns false when no config exists yet (backend 204)", async () => {
    getJiraConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useJiraAvailable("ws-1"));
    await waitFor(() => expect(getJiraConfigMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });
});
