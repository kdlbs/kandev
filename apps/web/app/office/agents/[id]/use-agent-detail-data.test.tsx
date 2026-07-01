import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { usePatchOfficeAgentProfileCache } from "./use-agent-detail-data";

const AGENT_TIMESTAMP = "2026-05-04T00:00:00Z";
const WORKSPACE_ID = "workspace-1";
const AGENT_ID = toAgentProfileId("agent-1");
const OTHER_AGENT_ID = toAgentProfileId("agent-2");

function agent(overrides: Partial<AgentProfile> = {}): AgentProfile {
  return {
    id: AGENT_ID,
    workspaceId: WORKSPACE_ID,
    name: "Agent",
    role: "worker",
    status: "idle",
    createdAt: AGENT_TIMESTAMP,
    updatedAt: AGENT_TIMESTAMP,
    permissions: {},
    budgetMonthlyCents: 5000,
    maxConcurrentSessions: 2,
    ...overrides,
  } as AgentProfile;
}

function renderPatchHook(workspaceId = WORKSPACE_ID) {
  const queryClient = makeQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <StateProvider initialState={{ workspaces: { activeId: workspaceId } }}>
        {children}
      </StateProvider>
    </QueryClientProvider>
  );

  return {
    ...renderHook(() => usePatchOfficeAgentProfileCache(), { wrapper }),
    queryClient,
  };
}

afterEach(() => {
  cleanup();
});

describe("usePatchOfficeAgentProfileCache", () => {
  it("patches cached agent profiles without immediately invalidating the list", () => {
    const { result, queryClient } = renderPatchHook();
    queryClient.setQueryData(qk.office.agents(WORKSPACE_ID), {
      agents: [agent(), agent({ id: OTHER_AGENT_ID, name: "Other" })],
    });

    act(() => result.current(AGENT_ID, { name: "Updated" }));

    expect(queryClient.getQueryData(qk.office.agents(WORKSPACE_ID))).toEqual({
      agents: [
        expect.objectContaining({ id: AGENT_ID, name: "Updated" }),
        agent({ id: OTHER_AGENT_ID, name: "Other" }),
      ],
    });
    expect(queryClient.getQueryState(qk.office.agents(WORKSPACE_ID))?.isInvalidated).toBe(false);
  });
});
