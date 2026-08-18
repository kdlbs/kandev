import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, AgentProfile } from "./types";
import { agentProfileId as toAgentProfileId, workspaceId as toWorkspaceId } from "@/lib/types/ids";

const WS = "ws-1";
const OTHER_WS = "ws-2";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

function agentsOf(store: ReturnType<typeof makeStore>, workspaceId: string): AgentProfile[] {
  return store.getState().office.agentProfilesByWorkspaceId[workspaceId] ?? [];
}

function makeAgent(id: string, name: string, workspace = WS): AgentProfile {
  return {
    id: toAgentProfileId(id),
    workspaceId: toWorkspaceId(workspace),
    name,
    role: "worker",
    status: "idle",
    budgetMonthlyCents: 1000,
    maxConcurrentSessions: 1,
    agentId: "claude",
    agentDisplayName: "Claude",
    model: "claude-sonnet-4-5",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("agent instance store actions", () => {
  it("setOfficeAgentProfiles replaces the list", () => {
    const store = makeStore();
    const agents = [makeAgent("a1", "Agent 1"), makeAgent("a2", "Agent 2")];
    store.getState().setOfficeAgentProfiles(WS, agents);
    expect(agentsOf(store, WS)).toHaveLength(2);
  });

  it("addOfficeAgentProfile appends to the list", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles(WS, [makeAgent("a1", "Agent 1")]);
    store.getState().addOfficeAgentProfile(WS, makeAgent("a2", "Agent 2"));
    expect(agentsOf(store, WS)).toHaveLength(2);
    expect(agentsOf(store, WS)[1].name).toBe("Agent 2");
  });

  it("addOfficeAgentProfile seeds a workspace that has no list yet", () => {
    const store = makeStore();
    store.getState().addOfficeAgentProfile(WS, makeAgent("a1", "Agent 1"));
    expect(agentsOf(store, WS)).toHaveLength(1);
  });

  it("updateOfficeAgentProfile patches an existing agent", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles(WS, [makeAgent("a1", "Original")]);
    store.getState().updateOfficeAgentProfile(WS, "a1", { name: "Updated", status: "working" });

    const agent = agentsOf(store, WS)[0];
    expect(agent.name).toBe("Updated");
    expect(agent.status).toBe("working");
    // Other fields unchanged
    expect(agent.role).toBe("worker");
  });

  it("updateOfficeAgentProfile is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles(WS, [makeAgent("a1", "Agent 1")]);
    store.getState().updateOfficeAgentProfile(WS, "unknown", { name: "Ghost" });
    expect(agentsOf(store, WS)).toHaveLength(1);
    expect(agentsOf(store, WS)[0].name).toBe("Agent 1");
  });

  it("updateOfficeAgentProfile is a no-op for a workspace with no list", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles(WS, [makeAgent("a1", "Agent 1")]);
    store.getState().updateOfficeAgentProfile(OTHER_WS, "a1", { name: "Ghost" });
    expect(agentsOf(store, WS)[0].name).toBe("Agent 1");
    expect(agentsOf(store, OTHER_WS)).toHaveLength(0);
  });

  it("removeOfficeAgentProfile removes by id", () => {
    const store = makeStore();
    store
      .getState()
      .setOfficeAgentProfiles(WS, [makeAgent("a1", "Agent 1"), makeAgent("a2", "Agent 2")]);
    store.getState().removeOfficeAgentProfile(WS, "a1");
    expect(agentsOf(store, WS)).toHaveLength(1);
    expect(agentsOf(store, WS)[0].id).toBe("a2");
  });

  it("removeOfficeAgentProfile is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles(WS, [makeAgent("a1", "Agent 1")]);
    store.getState().removeOfficeAgentProfile(WS, "unknown");
    expect(agentsOf(store, WS)).toHaveLength(1);
  });

  it("keeps each workspace's agents separate", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles(WS, [makeAgent("a1", "Agent 1")]);
    store.getState().setOfficeAgentProfiles(OTHER_WS, [makeAgent("b1", "Other 1", OTHER_WS)]);

    // The bug this keying exists to prevent: loading one workspace's agents
    // used to overwrite the other's, so a switch mid-flight rendered the wrong
    // list under the new workspace's name.
    store.getState().removeOfficeAgentProfile(WS, "a1");

    expect(agentsOf(store, WS)).toHaveLength(0);
    expect(agentsOf(store, OTHER_WS)).toHaveLength(1);
    expect(agentsOf(store, OTHER_WS)[0].name).toBe("Other 1");
  });
});
