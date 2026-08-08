import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { AgentRuntimeAvailability } from "@/lib/types/agent-runtime";
import { registerSystemEventsHandlers } from "./system-events";

const unavailable: AgentRuntimeAvailability = {
  status: "unavailable",
  reason: "agentctl_exited",
  occurred_at: "2026-08-08T14:22:52Z",
};

const available: AgentRuntimeAvailability = { status: "available" };

describe("registerSystemEventsHandlers", () => {
  it("replaces the complete runtime snapshot without touching domain state", () => {
    const store = createAppStore({
      agentRuntime: unavailable,
      tasks: {
        activeTaskId: "task-1",
        activeSessionId: null,
        pinnedSessionId: null,
        lastSessionByTaskId: {},
      },
    });
    const handlers = registerSystemEventsHandlers(store);

    handlers["system.agent_runtime.status_changed"]?.({
      type: "notification",
      action: "system.agent_runtime.status_changed",
      payload: available,
    });

    expect(store.getState().agentRuntime).toEqual(available);
    expect(store.getState().tasks.activeTaskId).toBe("task-1");
  });

  it("does not infer recovery from a connection status update", () => {
    const store = createAppStore({ agentRuntime: unavailable });

    store.getState().setConnectionStatus("connected");
    store.getState().setConnectionIssueSeverity("none");

    expect(store.getState().agentRuntime).toEqual(unavailable);
  });
});
