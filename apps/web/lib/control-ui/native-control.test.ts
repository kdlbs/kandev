import { describe, expect, it } from "vitest";
import {
  nativeDashboard,
  nativeHistory,
  nativeTasks,
  nativeLedger,
  nativeEvents,
} from "./native-control";
const window = {
  workspace_id: "w",
  since: "2026-10-18T03:00:00.123456789Z",
  generated_at: "2026-10-19T03:00:00.123456789Z",
};
describe("task3 native control contracts", () => {
  it("retains per-source unknown coverage and exact inclusive history boundaries", () => {
    const data = {
      ...window,
      coverage: "partial",
      truncated: false,
      sources: {
        server_start: {
          state: "unavailable",
          reason: "not_projected",
          total: null,
          returned: 0,
          truncated: false,
        },
      },
      events: [
        {
          id: "event",
          kind: "session_started",
          task_id: "task",
          session_id: "sid",
          occurred_at: window.since,
        },
      ],
    };
    expect(nativeHistory(data, "w")).toBe(true);
    expect(
      nativeHistory(
        { ...data, events: [{ ...data.events[0], occurred_at: "2026-10-18T03:00:00.123456788Z" }] },
        "w",
      ),
    ).toBe(false);
    expect(nativeHistory(data, "foreign")).toBe(false);
    if (nativeHistory(data, "w"))
      expect(nativeEvents(data)[0]).toMatchObject({
        id: "event",
        kind: "session_started",
        task_id: "task",
        session_id: "sid",
        workspace_id: "w",
      });
  });
  it("recognizes recorded dashboard zero without fabricating inference or omitted metrics", () => {
    const data = {
      ...window,
      basis: "recorded_task_session_queue_transition_state",
      inference_state: "unknown",
      tasks: {},
      sessions: {},
      waiting_input_tasks: 0,
      wip_waiting_tasks: 0,
      queued_messages: 0,
      transitions_24h: 0,
      completed_tasks_24h: 0,
    };
    expect(nativeDashboard(data, "w")).toBe(true);
    expect(nativeDashboard({ ...data, waiting_input_tasks: undefined }, "w")).toBe(false);
    expect(nativeDashboard({ ...data, sessions: { RUNNING: -1 } }, "w")).toBe(false);
    expect(nativeDashboard({ workspace_id: "w", agents: [] }, "w")).toBe(false);
  });
});

describe("native task and ledger projections", () => {
  it("requires exact filter/archive scope and retains updated time independently of completion", () => {
    const data = {
      kind: "tasks",
      filter: "completed",
      archive: "all",
      total: 1,
      truncated: false,
      generated_at: window.generated_at,
      tasks: [
        {
          task_id: "t",
          workspace_id: "w",
          title: "Task",
          state: "IN_PROGRESS",
          step_id: "step",
          step_name: "Develop",
          stage_type: "work",
          session_id: "sid",
          session_state: "RUNNING",
          archived: false,
          running_sessions: 1,
          queued_messages: 0,
          updated_at: window.generated_at,
        },
      ],
    };
    expect(nativeTasks(data, "w", "completed", "all")).toBe(true);
    expect(nativeTasks(data, "w", "all", "all")).toBe(false);
    expect(nativeTasks(data, "w", "completed", "active")).toBe(false);
    expect(nativeTasks({ ...data, total: 0 }, "w", "completed", "all")).toBe(false);
  });
  it("keeps recorded ledger null measurements and unknown attribution distinct from zero", () => {
    const model = {
      model: "",
      provider: "",
      measurement_source: "provider",
      usage_completeness: "exact",
      cost_source: "unpriced",
      estimated: false,
      event_count: 1,
      tokens_in: 0,
      tokens_out: null,
      tokens_cached_read: 0,
      tokens_cached_write: null,
      tokens_total: 0,
      cost_subcents: 0,
    };
    const data = {
      ...window,
      basis: "task_usage_events",
      coverage: "recorded_events_only",
      account_attribution: "unavailable",
      truncated: false,
      models: [model],
    };
    expect(nativeLedger(data, "w")).toBe(true);
    expect(nativeLedger([model], "w")).toBe(false);
    expect(
      nativeLedger(
        { ...data, models: [{ ...model, tokens_total: Number.MAX_SAFE_INTEGER + 1 }] },
        "w",
      ),
    ).toBe(false);
  });
});
