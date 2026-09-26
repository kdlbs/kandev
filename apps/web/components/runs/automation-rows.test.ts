import { t } from "@/lib/i18n";
import { describe, expect, it } from "vitest";
import type {
  Automation,
  AutomationTrigger,
  AutomationRun,
  AutomationSummary,
} from "@/lib/types/automation";
import {
  AUTOMATION_OFF_REASON_KEY,
  automationState,
  buildAutomationRows,
  concurrencyReason,
  NO_SCHEDULE_REASON_KEY,
  nextFiring,
  SCHEDULE_OFF_REASON_KEY,
} from "./automation-rows";

const SINGAPORE = "Asia/Singapore";
const MIDNIGHT_DAILY = "0 0 * * *";
const NOW = new Date("2026-07-31T10:00:00Z");
const CREATED_AT = "2026-07-01T00:00:00Z";
const AUTO_1 = "auto-1";

function mkTrigger(overrides: Partial<AutomationTrigger> = {}): AutomationTrigger {
  return {
    id: "trig-1",
    automation_id: AUTO_1,
    type: "scheduled",
    config: { cron_expression: MIDNIGHT_DAILY, timezone: SINGAPORE },
    enabled: true,
    last_evaluated_at: null,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
    ...overrides,
  };
}

function mkAutomation(overrides: Partial<Automation> = {}): Automation {
  return {
    id: AUTO_1,
    workspace_id: "ws-1",
    name: "Nightly drift sweep",
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
    triggers: [mkTrigger()],
    ...overrides,
  };
}

function mkRun(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "run-1",
    automation_id: AUTO_1,
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: "2026-07-30T16:00:00Z",
    ...overrides,
  };
}

describe("nextFiring", () => {
  it("resolves the firing when nothing is in the way", () => {
    const next = nextFiring(mkAutomation(), 0, NOW);

    expect(next.kind).toBe("time");
    expect(next.text).toBe("~00:00 tomorrow · GMT+8");
  });

  it("explains a disabled automation instead of promising a time", () => {
    const next = nextFiring(mkAutomation({ enabled: false }), 0, NOW);

    expect(next).toEqual({ kind: "reason", text: t(AUTOMATION_OFF_REASON_KEY) });
  });

  it("explains an automation with no schedule at all", () => {
    expect(nextFiring(mkAutomation({ triggers: [] }), 0, NOW)).toEqual({
      kind: "reason",
      text: t(NO_SCHEDULE_REASON_KEY),
    });
  });

  it("explains a schedule trigger that is switched off", () => {
    const off = mkAutomation({ triggers: [mkTrigger({ enabled: false })] });

    expect(nextFiring(off, 0, NOW)).toEqual({ kind: "reason", text: t(SCHEDULE_OFF_REASON_KEY) });
  });

  it("explains a firing blocked by the concurrency cap, naming the cap", () => {
    const next = nextFiring(mkAutomation({ max_concurrent_runs: 2 }), 2, NOW);

    expect(next.kind).toBe("reason");
    expect(next.text).toBe(concurrencyReason(2));
    expect(next.text).toContain("max 2 at a time");
  });

  it("says nothing about the cap when one run is open and one slot is configured", () => {
    // The default shape. Every run a single-slot automation ever does is "at
    // the cap" the moment it starts, so reporting that as Paused — in amber,
    // beside a dot already reading `running` — made the normal case look
    // broken. The cap is only news once it is actually queueing something.
    const next = nextFiring(mkAutomation({ max_concurrent_runs: 1 }), 1, NOW);

    expect(next.kind).toBe("time");
    expect(next.text).not.toContain("Paused");
  });

  it("still reports a single-slot automation that has somehow gone past its slot", () => {
    // Two open runs against a cap of one is a real jam, not the steady state.
    expect(nextFiring(mkAutomation({ max_concurrent_runs: 1 }), 2, NOW)).toEqual({
      kind: "reason",
      text: concurrencyReason(1),
    });
  });

  it("still resolves a time while below the cap", () => {
    expect(nextFiring(mkAutomation({ max_concurrent_runs: 3 }), 2, NOW).kind).toBe("time");
  });

  it("prefers the disabling fact over the schedule that is now irrelevant", () => {
    // A switched-off automation with an open run must not report the cap: the
    // cap is not why it will not fire.
    const off = mkAutomation({ enabled: false });

    expect(nextFiring(off, 1, NOW).text).toBe(t(AUTOMATION_OFF_REASON_KEY));
  });

  it("states the rule for an interval it cannot resolve to an instant", () => {
    const every = mkAutomation({
      triggers: [mkTrigger({ config: { cron_expression: "@every 15m" } })],
    });

    expect(nextFiring(every, 0, NOW)).toEqual({ kind: "time", text: "Every 15 minutes" });
  });
});

describe("automationState", () => {
  it("is running while a run is open, even at the concurrency cap", () => {
    expect(automationState(mkAutomation(), 1)).toBe("running");
  });

  it("is paused when the automation is switched off", () => {
    expect(automationState(mkAutomation({ enabled: false }), 0)).toBe("paused");
  });

  it("is paused when the schedule trigger is switched off", () => {
    expect(automationState(mkAutomation({ triggers: [mkTrigger({ enabled: false })] }), 0)).toBe(
      "paused",
    );
  });

  it("is idle when it is simply waiting for its next firing", () => {
    expect(automationState(mkAutomation(), 0)).toBe("idle");
  });

  it("is idle, not paused, when it has no schedule to be stopped", () => {
    // Nothing is holding it back — it just only runs on demand.
    expect(automationState(mkAutomation({ triggers: [] }), 0)).toBe("idle");
  });
});

function mkSummary(overrides: Partial<AutomationSummary> = {}): AutomationSummary {
  return { automation_id: AUTO_1, open_runs: 0, ...overrides };
}

describe("buildAutomationRows", () => {
  const OTHER = mkAutomation({ id: "auto-2", name: "Dependency audit", triggers: [] });

  it("gives every automation a row, including ones that have never run", () => {
    const rows = buildAutomationRows([mkAutomation(), OTHER], [], NOW);

    expect(rows.map((row) => row.automation.id).sort()).toEqual([AUTO_1, "auto-2"]);
    expect(rows.every((row) => row.lastRun === null)).toBe(true);
  });

  it("orders by most recent activity", () => {
    const rows = buildAutomationRows(
      [mkAutomation(), OTHER],
      [
        mkSummary({ last_run: mkRun({ id: "old", created_at: "2026-07-20T00:00:00Z" }) }),
        mkSummary({
          automation_id: "auto-2",
          last_run: mkRun({
            id: "new",
            automation_id: "auto-2",
            created_at: "2026-07-30T00:00:00Z",
          }),
        }),
      ],
      NOW,
    );

    expect(rows.map((row) => row.automation.id)).toEqual(["auto-2", AUTO_1]);
  });

  it("takes each automation's last run from its own summary", () => {
    const rows = buildAutomationRows(
      [mkAutomation()],
      [mkSummary({ last_run: mkRun({ id: "newest", created_at: "2026-07-30T00:00:00Z" }) })],
      NOW,
    );

    expect(rows[0].lastRun?.id).toBe("newest");
  });

  it("does not attribute another automation's summary", () => {
    const rows = buildAutomationRows(
      [mkAutomation()],
      [mkSummary({ automation_id: "auto-9", open_runs: 3, last_run: mkRun({ id: "foreign" }) })],
      NOW,
    );

    expect(rows[0].lastRun).toBeNull();
    expect(rows[0].state).toBe("idle");
  });

  it("reports a jammed automation as running with the cap as its reason", () => {
    const rows = buildAutomationRows(
      [mkAutomation({ max_concurrent_runs: 2 })],
      [mkSummary({ open_runs: 2, last_run: mkRun({ id: "open", status: "task_created" }) })],
      NOW,
    );

    expect(rows[0].state).toBe("running");
    expect(rows[0].next).toEqual({ kind: "reason", text: concurrencyReason(2) });
  });

  it("reports a single-slot automation with one run open as running, and says nothing else", () => {
    // The everyday case. `running` is the whole story; there is no second
    // amber sentence claiming the automation is Paused as well.
    const rows = buildAutomationRows(
      [mkAutomation({ max_concurrent_runs: 1 })],
      [mkSummary({ open_runs: 1, last_run: mkRun({ id: "open", status: "task_created" }) })],
      NOW,
    );

    expect(rows[0].state).toBe("running");
    expect(rows[0].next.kind).toBe("time");
  });

  it("trusts the server's open count rather than re-deriving it from statuses", () => {
    // The count is what the concurrency cap itself uses. A finished run left in
    // the summary must not make the row claim something is still going.
    const rows = buildAutomationRows(
      [mkAutomation({ max_concurrent_runs: 1 })],
      [mkSummary({ open_runs: 0, last_run: mkRun({ id: "done", status: "succeeded" }) })],
      NOW,
    );

    expect(rows[0].state).toBe("idle");
    expect(rows[0].next.kind).toBe("time");
  });

  it("reads a missing summary as an automation that has never run", () => {
    const rows = buildAutomationRows([mkAutomation()], [], NOW);

    expect(rows[0].lastRun).toBeNull();
    expect(rows[0].state).toBe("idle");
  });
});
