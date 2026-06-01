import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { StateProvider } from "@/components/state-provider";
import { createTestQueryClient } from "@/test-utils/render-with-query";
import { qk } from "@/lib/query/keys";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { useOfficeAgents, useAgentName, useAgentProfile } from "../use-office-agents";

const WS_ID = "ws-1";

const listAgentProfilesMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/office-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/office-api")>(
    "@/lib/api/domains/office-api",
  );
  return { ...actual, listAgentProfiles: listAgentProfilesMock };
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function makeAgent(id: string, name: string): AgentProfile {
  return {
    id,
    workspaceId: WS_ID,
    name,
    role: "worker",
    status: "idle",
    permissions: {},
    pauseReason: "",
    budgetMonthlyCents: 0,
    maxConcurrentSessions: 1,
    createdAt: "2026-05-01T00:00:00Z",
    updatedAt: "2026-05-01T00:00:00Z",
  } as AgentProfile;
}

function makeWrapper(activeId: string | null, seed?: AgentProfile[]) {
  const client = createTestQueryClient();
  if (seed) client.setQueryData(qk.office.agents(WS_ID), seed);
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <StateProvider initialState={{ workspaces: { activeId } }}>{children}</StateProvider>
      </QueryClientProvider>
    );
  }
  return { Wrapper, client };
}

describe("useOfficeAgents", () => {
  it("returns the seeded agents for the active workspace without fetching", () => {
    const agents = [makeAgent("a-1", "Alice"), makeAgent("a-2", "Bob")];
    const { Wrapper } = makeWrapper(WS_ID, agents);
    const { result } = renderHook(() => useOfficeAgents(), { wrapper: Wrapper });
    expect(result.current).toHaveLength(2);
    expect(listAgentProfilesMock).not.toHaveBeenCalled();
  });

  it("fetches agents when the cache is empty", async () => {
    listAgentProfilesMock.mockResolvedValue({ agents: [makeAgent("a-1", "Alice")] });
    const { Wrapper } = makeWrapper(WS_ID);
    const { result } = renderHook(() => useOfficeAgents(), { wrapper: Wrapper });
    await waitFor(() => expect(result.current).toHaveLength(1));
    expect(listAgentProfilesMock).toHaveBeenCalledWith(WS_ID);
  });

  it("returns an empty array and does not fetch when no workspace is active", () => {
    const { Wrapper } = makeWrapper(null);
    const { result } = renderHook(() => useOfficeAgents(), { wrapper: Wrapper });
    expect(result.current).toEqual([]);
    expect(listAgentProfilesMock).not.toHaveBeenCalled();
  });
});

describe("useAgentName", () => {
  it("resolves the agent name by id from the active workspace", () => {
    const { Wrapper } = makeWrapper(WS_ID, [makeAgent("a-1", "Alice")]);
    const { result } = renderHook(() => useAgentName("a-1"), { wrapper: Wrapper });
    expect(result.current).toBe("Alice");
  });

  it("returns undefined for an unknown or null id", () => {
    const { Wrapper } = makeWrapper(WS_ID, [makeAgent("a-1", "Alice")]);
    const unknown = renderHook(() => useAgentName("missing"), { wrapper: Wrapper });
    expect(unknown.result.current).toBeUndefined();
    const nullId = renderHook(() => useAgentName(null), { wrapper: Wrapper });
    expect(nullId.result.current).toBeUndefined();
  });
});

describe("useAgentProfile", () => {
  it("resolves the full profile by id", () => {
    const { Wrapper } = makeWrapper(WS_ID, [makeAgent("a-1", "Alice")]);
    const { result } = renderHook(() => useAgentProfile("a-1"), { wrapper: Wrapper });
    expect(result.current?.name).toBe("Alice");
  });

  it("returns undefined for a null id", () => {
    const { Wrapper } = makeWrapper(WS_ID, [makeAgent("a-1", "Alice")]);
    const { result } = renderHook(() => useAgentProfile(null), { wrapper: Wrapper });
    expect(result.current).toBeUndefined();
  });
});
