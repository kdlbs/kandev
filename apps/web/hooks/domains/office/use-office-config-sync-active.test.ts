import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { OfficeConfigSyncConfig } from "@/lib/types/office-config-sync";

const getOfficeConfigSyncConfigMock =
  vi.fn<(workspaceId: string) => Promise<OfficeConfigSyncConfig | null>>();
vi.mock("@/lib/api/domains/office-config-sync-api", () => ({
  getOfficeConfigSyncConfig: (workspaceId: string) => getOfficeConfigSyncConfigMock(workspaceId),
}));

import { useOfficeConfigSyncActive } from "./use-office-config-sync-active";

function makeConfig(): OfficeConfigSyncConfig {
  return {
    workspace_id: "ws-1",
    provider: "github",
    repo_owner: "kdlbs",
    repo_name: "kandev-office-config",
    project_path: "",
    branch: "main",
    path: "",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("useOfficeConfigSyncActive", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("is false while unconfigured", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useOfficeConfigSyncActive("ws-1"));
    await waitFor(() => expect(getOfficeConfigSyncConfigMock).toHaveBeenCalled());
    expect(result.current).toBe(false);
  });

  it("becomes true once a config sync source exists", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(makeConfig());
    const { result } = renderHook(() => useOfficeConfigSyncActive("ws-1"));
    await waitFor(() => expect(result.current).toBe(true));
  });

  it("stays false for an empty workspace id and makes no request", () => {
    const { result } = renderHook(() => useOfficeConfigSyncActive(""));
    expect(result.current).toBe(false);
    expect(getOfficeConfigSyncConfigMock).not.toHaveBeenCalled();
  });

  it("polls on the background refresh interval and picks up a later config", async () => {
    vi.useFakeTimers();
    getOfficeConfigSyncConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useOfficeConfigSyncActive("ws-1"));
    await vi.waitFor(() => expect(getOfficeConfigSyncConfigMock).toHaveBeenCalledTimes(1));
    expect(result.current).toBe(false);

    getOfficeConfigSyncConfigMock.mockResolvedValue(makeConfig());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(90_000);
    });
    expect(result.current).toBe(true);
  });
});
