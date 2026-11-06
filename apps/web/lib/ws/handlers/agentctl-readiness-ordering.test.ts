import { describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { registerTaskSessionHandlers } from "./agent-session";

const RESTART_TIME = "2026-10-06T02:37:40Z";
const ORIGINAL_TIME = "2026-10-06T02:37:39Z";

function expectNewRestartToRejectOldReadiness(): void {
  const store = createAppStore();
  const handlers = registerTaskSessionHandlers(store);
  store.getState().setSessionAgentctlStatus("session-1", {
    status: "ready",
    agentExecutionId: "old",
    updatedAt: ORIGINAL_TIME,
  });
  const invalidate = vi.fn(store.getState().invalidateConfirmedConfigOptions);
  store.setState({ invalidateConfirmedConfigOptions: invalidate });
  handlers["session.agentctl_starting"]!({
    type: "notification",
    action: "session.agentctl_starting",
    timestamp: RESTART_TIME,
    payload: { session_id: "session-1", agent_execution_id: "new" },
  } as never);
  expect(invalidate).toHaveBeenCalledExactlyOnceWith("session-1", "new");
  handlers["session.agentctl_ready"]!({
    type: "notification",
    action: "session.agentctl_ready",
    timestamp: "2026-10-06T02:37:39.5Z",
    payload: { session_id: "session-1", agent_execution_id: "old" },
  } as never);
  expect(store.getState().sessionAgentctl.itemsBySessionId["session-1"]).toMatchObject({
    status: "starting",
    agentExecutionId: "new",
  });
}

describe("agentctl observation ordering", () => {
  it.each([
    ["ready", "starting"],
    ["ready", "error"],
    ["error", "ready"],
  ] as const)("keeps newer %s when an older %s arrives", (current, delayed) => {
    const store = createAppStore();
    const handlers = registerTaskSessionHandlers(store);
    handlers[`session.agentctl_${current}`]!({
      type: "notification",
      action: `session.agentctl_${current}`,
      timestamp: "2026-10-06T02:37:39.000000002Z",
      payload: { session_id: "session-1", agent_execution_id: "execution-1" },
    } as never);
    // Session-status recovery can confirm readiness without observation metadata.
    store.getState().setSessionAgentctlStatus("session-1", { status: current });
    const upsert = vi.spyOn(store.getState(), "upsertTaskSessionFromEvent");
    const invalidate = vi.fn(store.getState().invalidateConfirmedConfigOptions);
    store.setState({ invalidateConfirmedConfigOptions: invalidate });
    handlers[`session.agentctl_${delayed}`]!({
      type: "notification",
      action: `session.agentctl_${delayed}`,
      timestamp: "2026-10-06T03:37:39.000000001+01:00",
      payload: {
        session_id: "session-1",
        agent_execution_id: "execution-old",
        task_id: "task-1",
        task_environment_id: "old-environment",
        workspace_path: "/obsolete",
      },
    } as never);
    expect(upsert).not.toHaveBeenCalled();
    expect(invalidate).not.toHaveBeenCalled();
    expect(store.getState().sessionAgentctl.itemsBySessionId["session-1"]).toMatchObject({
      status: current,
      agentExecutionId: "execution-1",
      updatedAt: "2026-10-06T02:37:39.000000002Z",
    });
  });

  it(
    "accepts a newer restart and rejects readiness from the previous execution",
    expectNewRestartToRejectOldReadiness,
  );

  it("does not promote a restarted execution from an older live-state snapshot", () => {
    const store = createAppStore();
    store.getState().setSessionAgentctlStatus("session-1", {
      status: "starting",
      agentExecutionId: "new",
      updatedAt: RESTART_TIME,
    });
    registerTaskSessionHandlers(store)["session.state_changed"]!({
      type: "notification",
      action: "session.state_changed",
      timestamp: ORIGINAL_TIME,
      payload: {
        session_id: "session-1",
        task_id: "task-1",
        new_state: "RUNNING",
        updated_at: ORIGINAL_TIME,
      },
    } as never);
    expect(store.getState().sessionAgentctl.itemsBySessionId["session-1"].status).toBe("starting");
  });

  it.each([undefined, "invalid", "2026-10-06T03:37:39+01:00"])(
    "accepts legacy or equal-time observations: %s",
    (timestamp) => {
      const store = createAppStore();
      store.getState().setSessionAgentctlStatus("session-1", {
        status: "ready",
        updatedAt: ORIGINAL_TIME,
      });
      registerTaskSessionHandlers(store)["session.agentctl_starting"]!({
        type: "notification",
        action: "session.agentctl_starting",
        timestamp,
        payload: { session_id: "session-1", agent_execution_id: "restart" },
      } as never);
      expect(store.getState().sessionAgentctl.itemsBySessionId["session-1"].status).toBe(
        "starting",
      );
    },
  );
});
