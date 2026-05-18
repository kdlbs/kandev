import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { registerOfficeHandlers } from "./office";

/**
 * Minimal in-memory store for the office WS handler tests.
 * Focus: workspace_id filtering in the handlers.
 */
function makeStore(activeWorkspaceId: string | null) {
  const setOfficeRefetchTrigger = vi.fn();
  const updateOfficeAgentProfile = vi.fn();
  const upsertProviderHealth = vi.fn();
  const appendRunAttempt = vi.fn();
  const setWorkspaceRouting = vi.fn();
  let state = {
    workspaces: { items: [], activeId: activeWorkspaceId },
    office: { tasks: { items: [] } },
    setOfficeRefetchTrigger,
    updateOfficeAgentProfile,
    upsertProviderHealth,
    appendRunAttempt,
    setWorkspaceRouting,
  } as unknown as AppState;
  const subs: Array<(s: AppState) => void> = [];
  const store: StoreApi<AppState> = {
    getState: () => state,
    setState: (next: AppState | ((s: AppState) => Partial<AppState>)) => {
      const partial =
        typeof next === "function" ? (next as (s: AppState) => Partial<AppState>)(state) : next;
      state = { ...state, ...partial } as AppState;
      subs.forEach((s) => s(state));
    },
    subscribe: (listener: (s: AppState) => void) => {
      subs.push(listener);
      return () => {
        const i = subs.indexOf(listener);
        if (i >= 0) subs.splice(i, 1);
      };
    },
    getInitialState: () => state,
  } as unknown as StoreApi<AppState>;
  return {
    store,
    setOfficeRefetchTrigger,
    updateOfficeAgentProfile,
    upsertProviderHealth,
    appendRunAttempt,
    setWorkspaceRouting,
  };
}

const ACTIVE_WS = "ws-current";

describe("office WS handler — workspace filter", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("triggers refetch when event workspace matches active workspace", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.task.created"]!;

    handler({
      type: "notification",
      action: "office.task.created",
      payload: { workspace_id: ACTIVE_WS, task_id: "t-1" },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("tasks");
    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("dashboard");
  });

  it("does NOT trigger refetch when event workspace differs from active workspace", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.task.created"]!;

    handler({
      type: "notification",
      action: "office.task.created",
      payload: { workspace_id: "ws-other", task_id: "t-1" },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).not.toHaveBeenCalled();
  });

  it("triggers refetch for legacy events without workspace_id (backwards compat)", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.task.created"]!;

    handler({
      type: "notification",
      action: "office.task.created",
      payload: { task_id: "t-1" },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("tasks");
    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("dashboard");
  });

  it("filters office.agent.completed by workspace too", () => {
    const { store, setOfficeRefetchTrigger, updateOfficeAgentProfile } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.agent.completed"]!;

    handler({
      type: "notification",
      action: "office.agent.completed",
      payload: { workspace_id: "ws-other", agent_profile_id: "agent-1" },
    } as Parameters<typeof handler>[0]);

    expect(updateOfficeAgentProfile).not.toHaveBeenCalled();
    expect(setOfficeRefetchTrigger).not.toHaveBeenCalled();
  });

  it("filters office.approval.created by workspace too", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.approval.created"]!;

    handler({
      type: "notification",
      action: "office.approval.created",
      payload: { workspace_id: "ws-other" },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).not.toHaveBeenCalled();
  });
});

describe("office WS handler — approval flow events", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("refreshes per-task DTO and inbox on office.task.decision_recorded", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.task.decision_recorded"]!;

    handler({
      type: "notification",
      action: "office.task.decision_recorded",
      payload: {
        workspace_id: ACTIVE_WS,
        task_id: "t-42",
        decision_id: "d-1",
        role: "approver",
        decider_type: "user",
        decider_id: "user",
        decision: "approved",
      },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("task:t-42");
    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("inbox");
  });

  it("refreshes the inbox on office.task.review_requested", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.task.review_requested"]!;

    handler({
      type: "notification",
      action: "office.task.review_requested",
      payload: {
        workspace_id: ACTIVE_WS,
        task_id: "t-99",
        role: "approver",
        reviewer_agent_id: "agent-x",
      },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("inbox");
    expect(setOfficeRefetchTrigger).toHaveBeenCalledWith("task:t-99");
  });

  it("does not refresh on cross-workspace office.task.decision_recorded", () => {
    const { store, setOfficeRefetchTrigger } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.task.decision_recorded"]!;

    handler({
      type: "notification",
      action: "office.task.decision_recorded",
      payload: { workspace_id: "ws-other", task_id: "t-1" },
    } as Parameters<typeof handler>[0]);

    expect(setOfficeRefetchTrigger).not.toHaveBeenCalled();
  });
});

describe("office WS handler — provider routing events", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("upserts provider health on office.provider.health_changed", () => {
    const { store, upsertProviderHealth } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.provider.health_changed"]!;

    handler({
      type: "notification",
      action: "office.provider.health_changed",
      payload: {
        workspace_id: ACTIVE_WS,
        provider_id: "claude-acp",
        scope: "provider",
        scope_value: "",
        state: "degraded",
        backoff_step: 1,
        error_code: "quota_limited",
      },
    } as Parameters<typeof handler>[0]);

    expect(upsertProviderHealth).toHaveBeenCalledWith(
      ACTIVE_WS,
      expect.objectContaining({
        provider_id: "claude-acp",
        state: "degraded",
        error_code: "quota_limited",
      }),
    );
  });

  it("appends route attempt on office.route_attempt.appended", () => {
    const { store, appendRunAttempt } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.route_attempt.appended"]!;

    handler({
      type: "notification",
      action: "office.route_attempt.appended",
      payload: {
        workspace_id: ACTIVE_WS,
        run_id: "run-1",
        attempt: {
          seq: 1,
          provider_id: "codex-acp",
          model: "gpt-5.4",
          tier: "balanced",
          outcome: "launched",
          started_at: "2026-05-10T12:00:00Z",
        },
      },
    } as Parameters<typeof handler>[0]);

    expect(appendRunAttempt).toHaveBeenCalledWith(
      "run-1",
      expect.objectContaining({ seq: 1, provider_id: "codex-acp" }),
    );
  });

  it("invalidates routing config on office.routing.settings_updated", () => {
    const { store, setWorkspaceRouting } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.routing.settings_updated"]!;

    handler({
      type: "notification",
      action: "office.routing.settings_updated",
      payload: { workspace_id: ACTIVE_WS },
    } as Parameters<typeof handler>[0]);

    expect(setWorkspaceRouting).toHaveBeenCalledWith(ACTIVE_WS, undefined);
  });

  it("filters provider health changes to current workspace", () => {
    const { store, upsertProviderHealth } = makeStore(ACTIVE_WS);
    const handlers = registerOfficeHandlers(store);
    const handler = handlers["office.provider.health_changed"]!;

    handler({
      type: "notification",
      action: "office.provider.health_changed",
      payload: {
        workspace_id: "ws-other",
        provider_id: "claude-acp",
        scope: "provider",
        scope_value: "",
        state: "degraded",
        backoff_step: 0,
      },
    } as Parameters<typeof handler>[0]);

    expect(upsertProviderHealth).not.toHaveBeenCalled();
  });
});
