import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOrchestrateSlice } from "./orchestrate-slice";
import type { OrchestrateSlice, AgentInstance } from "./types";

function makeStore() {
  return create<OrchestrateSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOrchestrateSlice as any)(...a) })),
  );
}

function makeAgent(id: string, name: string): AgentInstance {
  return {
    id,
    workspaceId: "ws-1",
    name,
    role: "worker",
    status: "idle",
    budgetMonthlyCents: 1000,
    maxConcurrentSessions: 1,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("agent instance store actions", () => {
  it("setAgentInstances replaces the list", () => {
    const store = makeStore();
    const agents = [makeAgent("a1", "Agent 1"), makeAgent("a2", "Agent 2")];
    store.getState().setAgentInstances(agents);
    expect(store.getState().orchestrate.agentInstances).toHaveLength(2);
  });

  it("addAgentInstance appends to the list", () => {
    const store = makeStore();
    store.getState().setAgentInstances([makeAgent("a1", "Agent 1")]);
    store.getState().addAgentInstance(makeAgent("a2", "Agent 2"));
    expect(store.getState().orchestrate.agentInstances).toHaveLength(2);
    expect(store.getState().orchestrate.agentInstances[1].name).toBe("Agent 2");
  });

  it("updateAgentInstance patches an existing agent", () => {
    const store = makeStore();
    store.getState().setAgentInstances([makeAgent("a1", "Original")]);
    store.getState().updateAgentInstance("a1", { name: "Updated", status: "working" });

    const agent = store.getState().orchestrate.agentInstances[0];
    expect(agent.name).toBe("Updated");
    expect(agent.status).toBe("working");
    // Other fields unchanged
    expect(agent.role).toBe("worker");
  });

  it("updateAgentInstance is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setAgentInstances([makeAgent("a1", "Agent 1")]);
    store.getState().updateAgentInstance("unknown", { name: "Ghost" });
    expect(store.getState().orchestrate.agentInstances).toHaveLength(1);
    expect(store.getState().orchestrate.agentInstances[0].name).toBe("Agent 1");
  });

  it("removeAgentInstance removes by id", () => {
    const store = makeStore();
    store.getState().setAgentInstances([
      makeAgent("a1", "Agent 1"),
      makeAgent("a2", "Agent 2"),
    ]);
    store.getState().removeAgentInstance("a1");
    expect(store.getState().orchestrate.agentInstances).toHaveLength(1);
    expect(store.getState().orchestrate.agentInstances[0].id).toBe("a2");
  });

  it("removeAgentInstance is a no-op for unknown id", () => {
    const store = makeStore();
    store.getState().setAgentInstances([makeAgent("a1", "Agent 1")]);
    store.getState().removeAgentInstance("unknown");
    expect(store.getState().orchestrate.agentInstances).toHaveLength(1);
  });
});
