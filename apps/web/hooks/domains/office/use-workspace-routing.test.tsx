import { describe, expect, it, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { qk } from "@/lib/query/keys";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";
import { useWorkspaceRouting } from "./use-workspace-routing";

const mocks = vi.hoisted(() => ({
  getWorkspaceRouting: vi.fn(),
  retryProvider: vi.fn(),
  updateWorkspaceRouting: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-extended-api", () => ({
  retryProvider: mocks.retryProvider,
  updateWorkspaceRouting: mocks.updateWorkspaceRouting,
}));

vi.mock("@/lib/api/domains/office-routing-api", () => ({
  getWorkspaceRouting: mocks.getWorkspaceRouting,
}));

const WORKSPACE_ID = "ws-1";
const PROVIDER_ID = "claude-acp";

describe("useWorkspaceRouting", () => {
  beforeEach(() => {
    mocks.getWorkspaceRouting.mockReset();
    mocks.updateWorkspaceRouting.mockReset();
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: {
        enabled: false,
        provider_order: [],
        default_tier: "balanced",
        provider_profiles: {},
      },
      known_providers: [PROVIDER_ID],
      execution_profiles: [
        { id: "profile-1", name: "Claude", provider_id: PROVIDER_ID, model: "sonnet" },
      ],
    });
  });

  it("fetches once on mount when there is no cached config", async () => {
    const { wrapper } = createQueryWrapper();
    const { unmount } = renderHook(() => useWorkspaceRouting(WORKSPACE_ID), {
      wrapper,
    });
    await waitFor(() => expect(mocks.getWorkspaceRouting).toHaveBeenCalledTimes(1));
    unmount();
  });

  it("updates the routing query cache without a store mirror", async () => {
    const { client, wrapper } = createQueryWrapper();
    const nextConfig: WorkspaceRouting = {
      enabled: true,
      provider_order: [PROVIDER_ID],
      default_tier: "balanced",
      provider_profiles: {},
    };

    const { result } = renderHook(() => useWorkspaceRouting(WORKSPACE_ID), { wrapper });

    await waitFor(() => expect(result.current.knownProviders).toEqual([PROVIDER_ID]));
    await act(async () => {
      await result.current.update(nextConfig);
    });

    expect(mocks.updateWorkspaceRouting).toHaveBeenCalledWith(WORKSPACE_ID, nextConfig);
    expect(client.getQueryData(qk.office.routing(WORKSPACE_ID))).toEqual({
      config: nextConfig,
      known_providers: [PROVIDER_ID],
      execution_profiles: [
        { id: "profile-1", name: "Claude", provider_id: PROVIDER_ID, model: "sonnet" },
      ],
    });
  });

  it("returns execution profiles from the routing query", async () => {
    const { wrapper } = createQueryWrapper();
    const { result } = renderHook(() => useWorkspaceRouting(WORKSPACE_ID), { wrapper });

    await waitFor(() => expect(result.current.executionProfiles).toHaveLength(1));
    expect(result.current.executionProfiles[0]).toMatchObject({
      id: "profile-1",
      provider_id: PROVIDER_ID,
    });
  });

  it("does not call setInterval (no polling)", () => {
    const spy = vi.spyOn(globalThis, "setInterval");
    const { wrapper } = createQueryWrapper();
    const { unmount } = renderHook(() => useWorkspaceRouting(WORKSPACE_ID), {
      wrapper,
    });
    expect(spy).not.toHaveBeenCalled();
    unmount();
    spy.mockRestore();
  });
});

function createQueryWrapper() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  const wrapper = function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
  return { client, wrapper };
}
