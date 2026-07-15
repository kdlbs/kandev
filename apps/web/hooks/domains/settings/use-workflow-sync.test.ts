import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";

const mockToast = vi.fn();
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

const getWorkflowSyncConfigMock = vi.fn<(opts?: unknown) => Promise<WorkflowSyncConfig | null>>();
const setWorkflowSyncConfigMock = vi.fn<(payload: unknown, opts?: unknown) => Promise<unknown>>();
const deleteWorkflowSyncConfigMock = vi.fn<(opts?: unknown) => Promise<unknown>>();
const forceWorkflowSyncMock = vi.fn<(opts?: unknown) => Promise<unknown>>();

vi.mock("@/lib/api/domains/workflow-sync-api", () => ({
  getWorkflowSyncConfig: (opts?: unknown) => getWorkflowSyncConfigMock(opts),
  setWorkflowSyncConfig: (payload: unknown, opts?: unknown) =>
    setWorkflowSyncConfigMock(payload, opts),
  deleteWorkflowSyncConfig: (opts?: unknown) => deleteWorkflowSyncConfigMock(opts),
  forceWorkflowSync: (opts?: unknown) => forceWorkflowSyncMock(opts),
}));

import { useWorkflowSync } from "./use-workflow-sync";

function makeConfig(overrides: Partial<WorkflowSyncConfig> = {}): WorkflowSyncConfig {
  return {
    workspace_id: "ws-1",
    repo_owner: "kdlbs",
    repo_name: "kandev",
    branch: "main",
    path: ".kandev/workflows",
    interval_seconds: 300,
    last_ok: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("useWorkflowSync", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getWorkflowSyncConfigMock.mockResolvedValue(null);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads config and pre-fills the form on mount", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ repo_owner: "acme", branch: "dev" }));
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config?.repo_owner).toBe("acme");
    expect(result.current.form.repo_owner).toBe("acme");
    expect(result.current.form.branch).toBe("dev");
  });

  it("falls back to defaults when unconfigured (204/null)", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config).toBeNull();
    expect(result.current.form.branch).toBe("main");
    expect(result.current.form.path).toBe(".kandev/workflows");
  });

  it("handleSave posts the form and updates config from the response", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(null);
    const saved = makeConfig({ repo_owner: "kdlbs", repo_name: "kandev" });
    setWorkflowSyncConfigMock.mockResolvedValue(saved);
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.update("repo_owner", "kdlbs"));
    act(() => result.current.update("repo_name", "kandev"));
    await act(async () => {
      await result.current.handleSave();
    });

    expect(setWorkflowSyncConfigMock).toHaveBeenCalledWith(
      expect.objectContaining({ repo_owner: "kdlbs", repo_name: "kandev" }),
      { workspaceId: "ws-1" },
    );
    expect(result.current.config?.repo_owner).toBe("kdlbs");
  });

  it("handleSyncNow updates config from the force-sync response, including failures", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    forceWorkflowSyncMock.mockResolvedValue({
      config: makeConfig({ last_ok: false, last_error: "clone failed" }),
      error: "clone failed",
    });
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSyncNow();
    });

    expect(forceWorkflowSyncMock).toHaveBeenCalledWith({ workspaceId: "ws-1" });
    expect(result.current.config?.last_ok).toBe(false);
    expect(result.current.config?.last_error).toBe("clone failed");
    expect(result.current.syncing).toBe(false);
  });

  it("handleDelete clears config and resets the form when confirmed", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig());
    deleteWorkflowSyncConfigMock.mockResolvedValue({ deleted: true });
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleDelete();
    });

    expect(deleteWorkflowSyncConfigMock).toHaveBeenCalledWith({ workspaceId: "ws-1" });
    expect(result.current.config).toBeNull();
    expect(result.current.form.repo_owner).toBe("");
    vi.unstubAllGlobals();
  });

  it("polls getWorkflowSyncConfig on the background refresh interval", async () => {
    vi.useFakeTimers();
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await vi.waitFor(() => expect(result.current.loading).toBe(false));
    expect(getWorkflowSyncConfigMock).toHaveBeenCalledTimes(1);

    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: false, last_error: "boom" }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(90_000);
    });
    expect(getWorkflowSyncConfigMock.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(result.current.config?.last_error).toBe("boom");
  });
});
