import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice, AgentProfile } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

function makeAgent(id: string, name: string): AgentProfile {
  return {
    id,
    workspaceId: "ws-1",
    name,
    role: "worker",
    status: "idle",
    budgetMonthlyCents: 1000,
    maxConcurrentSessions: 1,
    agentId: "claude",
    agentDisplayName: "Claude",
    model: "claude-sonnet-4-5",
    allowIndexing: false,
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
    store.getState().setOfficeAgentProfiles(agents);
    expect(store.getState().office.agentProfiles).toHaveLength(2);
  });

  it("addOfficeAgentProfile appends to the list", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles([makeAgent("a1", "Agent 1")]);
    store.getState().addOfficeAgentProfile(makeAgent("a2", "Agent 2"));
    expect(store.getState().office.agentProfiles).toHaveLength(2);
    expect(store.getState().office.agentProfiles[1].name).toBe("Agent 2");
  });

  it("updateOfficeAgentProfile patches an existing agent", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles([makeAgent("a1", "Original")]);
    store.getState().updateOfficeAgentProfile("a1", { name: "Updated", status: "working" });

    const agent = store.getState().office.agentProfiles[0];
    expect(agent.name).toBe("Updated");
    expect(agent.status).toBe("working");
    // Other fields unchanged
    expect(agent.role).toBe("worker");
  });

  it("updateOfficeAgentProfile is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles([makeAgent("a1", "Agent 1")]);
    store.getState().updateOfficeAgentProfile("unknown", { name: "Ghost" });
    expect(store.getState().office.agentProfiles).toHaveLength(1);
    expect(store.getState().office.agentProfiles[0].name).toBe("Agent 1");
  });

  it("removeOfficeAgentProfile removes by id", () => {
    const store = makeStore();
    store
      .getState()
      .setOfficeAgentProfiles([makeAgent("a1", "Agent 1"), makeAgent("a2", "Agent 2")]);
    store.getState().removeOfficeAgentProfile("a1");
    expect(store.getState().office.agentProfiles).toHaveLength(1);
    expect(store.getState().office.agentProfiles[0].id).toBe("a2");
  });

  it("removeOfficeAgentProfile is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setOfficeAgentProfiles([makeAgent("a1", "Agent 1")]);
    store.getState().removeOfficeAgentProfile("unknown");
    expect(store.getState().office.agentProfiles).toHaveLength(1);
  });
});
