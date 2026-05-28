import { describe, it, expect, vi, beforeEach } from "vitest";
import { waitFor } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import { useWorkspaceRouting } from "../use-workspace-routing";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";

const mocks = vi.hoisted(() => ({
  getWorkspaceRouting: vi.fn<
    [string],
    Promise<{ config: WorkspaceRouting | null; known_providers: string[] }>
  >(),
  updateWorkspaceRouting: vi.fn<[string, WorkspaceRouting], Promise<{ ok: boolean }>>(),
  retryProvider: vi.fn<[string, string], Promise<{ status: string }>>(),
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getWorkspaceRouting: mocks.getWorkspaceRouting,
  updateWorkspaceRouting: mocks.updateWorkspaceRouting,
  retryProvider: mocks.retryProvider,
}));

const WS_NAME = "ws-1";
const PROVIDER = "claude-acp";

const baseConfig: WorkspaceRouting = {
  enabled: true,
  provider_order: [PROVIDER],
  default_tier: "balanced",
  provider_profiles: {},
};

describe("useWorkspaceRouting", () => {
  beforeEach(() => {
    mocks.getWorkspaceRouting.mockReset();
    mocks.updateWorkspaceRouting.mockReset();
    mocks.retryProvider.mockReset();
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: baseConfig,
      known_providers: [PROVIDER],
    });
    mocks.updateWorkspaceRouting.mockResolvedValue({ ok: true });
    mocks.retryProvider.mockResolvedValue({ status: "ok" });
  });

  it("returns undefined config when workspaceName is null", () => {
    const { result } = renderHookWithQueryClient(() => useWorkspaceRouting(null));
    expect(result.current.config).toBeUndefined();
    expect(result.current.isLoading).toBe(false);
  });

  it("fetches and exposes config", async () => {
    const { result } = renderHookWithQueryClient(() => useWorkspaceRouting(WS_NAME));
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.config).toEqual(baseConfig);
    expect(result.current.knownProviders).toEqual([PROVIDER]);
  });

  it("does not poll (no setInterval)", () => {
    const spy = vi.spyOn(globalThis, "setInterval");
    const { unmount } = renderHookWithQueryClient(() => useWorkspaceRouting(WS_NAME));
    expect(spy).not.toHaveBeenCalled();
    unmount();
    spy.mockRestore();
  });

  it("update calls the API", async () => {
    const { result } = renderHookWithQueryClient(() => useWorkspaceRouting(WS_NAME));
    await waitFor(() => expect(result.current.config).toBeDefined());
    const newCfg: WorkspaceRouting = { ...baseConfig, enabled: false };
    await result.current.update(newCfg);
    expect(mocks.updateWorkspaceRouting).toHaveBeenCalledWith(WS_NAME, newCfg);
  });

  it("retry calls the API", async () => {
    const { result } = renderHookWithQueryClient(() => useWorkspaceRouting(WS_NAME));
    await waitFor(() => expect(result.current.config).toBeDefined());
    await result.current.retry(PROVIDER);
    expect(mocks.retryProvider).toHaveBeenCalledWith(WS_NAME, PROVIDER);
  });
});
