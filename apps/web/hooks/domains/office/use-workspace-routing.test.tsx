import { describe, expect, it, vi, beforeEach } from "vitest";
import { waitFor } from "@testing-library/react";
import { renderHookWithQueryClient } from "@/test-utils/render-with-query";
import { useWorkspaceRouting } from "./use-workspace-routing";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";

const mocks = vi.hoisted(() => ({
  getWorkspaceRouting: vi.fn<
    [string],
    Promise<{ config: WorkspaceRouting | null; known_providers: string[] }>
  >(),
  retryProvider: vi.fn<[string, string], Promise<{ status: string }>>(),
  updateWorkspaceRouting: vi.fn<[string, WorkspaceRouting], Promise<{ ok: boolean }>>(),
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getWorkspaceRouting: mocks.getWorkspaceRouting,
  retryProvider: mocks.retryProvider,
  updateWorkspaceRouting: mocks.updateWorkspaceRouting,
}));

describe("useWorkspaceRouting", () => {
  beforeEach(() => {
    mocks.getWorkspaceRouting.mockReset();
    mocks.updateWorkspaceRouting.mockReset();
    mocks.retryProvider.mockReset();
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: {
        enabled: false,
        provider_order: [],
        default_tier: "balanced",
        provider_profiles: {},
      },
      known_providers: ["claude-acp"],
    });
    mocks.updateWorkspaceRouting.mockResolvedValue({ ok: true });
    mocks.retryProvider.mockResolvedValue({ status: "ok" });
  });

  it("fetches once on mount when there is no cached config", async () => {
    const { unmount } = renderHookWithQueryClient(() => useWorkspaceRouting("ws-1"));
    await waitFor(() => expect(mocks.getWorkspaceRouting).toHaveBeenCalledTimes(1));
    unmount();
  });

  it("exposes known providers from the response", async () => {
    const { result, unmount } = renderHookWithQueryClient(() => useWorkspaceRouting("ws-1"));
    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.knownProviders).toEqual(["claude-acp"]);
    unmount();
  });

  it("does not call setInterval (no polling)", () => {
    const spy = vi.spyOn(globalThis, "setInterval");
    const { unmount } = renderHookWithQueryClient(() => useWorkspaceRouting("ws-1"));
    expect(spy).not.toHaveBeenCalled();
    unmount();
    spy.mockRestore();
  });
});
