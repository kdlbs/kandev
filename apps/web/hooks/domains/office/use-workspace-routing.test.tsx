import { describe, expect, it, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
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

const setKnownProviders = vi.fn();
const setWorkspaceRouting = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (state: unknown) => unknown) =>
    sel({
      office: {
        routing: { byWorkspace: {}, knownProviders: [] },
      },
      setKnownProviders,
      setWorkspaceRouting,
    }),
}));

describe("useWorkspaceRouting", () => {
  beforeEach(() => {
    setKnownProviders.mockReset();
    setWorkspaceRouting.mockReset();
    mocks.getWorkspaceRouting.mockReset();
    mocks.getWorkspaceRouting.mockResolvedValue({
      config: {
        enabled: false,
        provider_order: [],
        default_tier: "balanced",
        provider_profiles: {},
      },
      known_providers: ["claude-acp"],
    });
  });

  it("fetches once on mount when there is no cached config", async () => {
    const { unmount } = renderHook(() => useWorkspaceRouting("ws-1"), {
      wrapper: createQueryWrapper(),
    });
    await waitFor(() => expect(mocks.getWorkspaceRouting).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(setKnownProviders).toHaveBeenCalled());
    expect(setWorkspaceRouting).toHaveBeenCalled();
    unmount();
  });

  it("does not call setInterval (no polling)", () => {
    const spy = vi.spyOn(globalThis, "setInterval");
    const { unmount } = renderHook(() => useWorkspaceRouting("ws-1"), {
      wrapper: createQueryWrapper(),
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
  return function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}
