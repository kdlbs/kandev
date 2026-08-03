import { describe, expect, it } from "vitest";
import type { Automation, AutomationTrigger } from "@/lib/types/automation";
import {
  describeAutomationSchedule,
  formatNextFiring,
  nextFiringInstant,
  scheduleBinding,
} from "./automation-schedule";

const SINGAPORE = "Asia/Singapore";
const MIDNIGHT_DAILY = "0 0 * * *";
const CREATED_AT = "2026-07-01T00:00:00Z";
/** 18:00 in Singapore, so "tomorrow" and "today" differ by zone from here. */
const NOW = new Date("2026-07-31T10:00:00Z");

function mkTrigger(overrides: Partial<AutomationTrigger> = {}): AutomationTrigger {
  return {
    id: "trig-1",
    automation_id: "auto-1",
    type: "scheduled",
    config: { cron_expression: MIDNIGHT_DAILY, timezone: SINGAPORE },
    enabled: true,
    last_evaluated_at: null,
    created_at: CREATED_AT,
    updated_at: CREATED_AT,
    ...overrides,
  };
}

function mkAutomation(triggers: AutomationTrigger[]): Automation {
  return {
    id: "auto-1",
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
    triggers,
  };
}

describe("scheduleBinding", () => {
  it("reads the scheduled trigger's expression and zone", () => {
    const binding = scheduleBinding(mkAutomation([mkTrigger()]));

    expect(binding.expression).toBe(MIDNIGHT_DAILY);
    expect(binding.timeZone).toBe(SINGAPORE);
    expect(binding.trigger?.id).toBe("trig-1");
  });

  it("ignores non-scheduled triggers", () => {
    const binding = scheduleBinding(
      mkAutomation([mkTrigger({ type: "webhook", config: { filter_expression: "" } })]),
    );

    expect(binding.trigger).toBeNull();
    expect(binding.expression).toBe("");
  });

  it("treats a config with no usable cron as no schedule rather than throwing", () => {
    // config is an untyped bag on the wire; a hand-edited or legacy trigger can
    // carry anything, and the whole list must survive it.
    const binding = scheduleBinding(mkAutomation([mkTrigger({ config: { cron_expression: 42 } })]));

    expect(binding.expression).toBe("");
    expect(binding.timeZone).toBe("UTC");
  });

  it("defaults an absent timezone to UTC, matching the backend", () => {
    const binding = scheduleBinding(
      mkAutomation([mkTrigger({ config: { cron_expression: MIDNIGHT_DAILY } })]),
    );

    expect(binding.timeZone).toBe("UTC");
  });
});

describe("describeAutomationSchedule", () => {
  it("states the rule in words including the time", () => {
    expect(describeAutomationSchedule(mkAutomation([mkTrigger()]))).toBe(
      "Every day at 00:00 GMT+8",
    );
  });

  it("says there is no schedule when none is set", () => {
    expect(describeAutomationSchedule(mkAutomation([]))).toBe("No schedule");
  });
});

describe("nextFiringInstant", () => {
  it("resolves the next wall-clock firing in the schedule's own zone", () => {
    // 2026-07-31T10:00Z is 18:00 in Singapore, so midnight lands on 1 Aug local
    // = 2026-07-31T16:00Z.
    const at = nextFiringInstant(mkAutomation([mkTrigger()]), NOW);

    expect(at?.toISOString()).toBe("2026-07-31T16:00:00.000Z");
  });

  it("has nothing to resolve for a server-anchored interval", () => {
    const every = mkAutomation([mkTrigger({ config: { cron_expression: "@every 15m" } })]);

    expect(nextFiringInstant(every, NOW)).toBeNull();
  });

  it("has nothing to resolve without a schedule", () => {
    expect(nextFiringInstant(mkAutomation([]))).toBeNull();
  });
});

describe("formatNextFiring", () => {
  it("marks the time as approximate and names the zone", () => {
    const at = new Date("2026-07-31T16:00:00Z"); // 00:00 on 1 Aug in Singapore

    expect(formatNextFiring(at, SINGAPORE, NOW)).toBe("~00:00 tomorrow · GMT+8");
  });

  it("says today when the firing is still ahead on the same local day", () => {
    const at = new Date("2026-07-31T14:00:00Z"); // 22:00 same day in Singapore

    expect(formatNextFiring(at, SINGAPORE, NOW)).toBe("~22:00 today · GMT+8");
  });

  it("names the weekday within the week", () => {
    const at = new Date("2026-08-02T01:00:00Z"); // 09:00 Sunday in Singapore

    expect(formatNextFiring(at, SINGAPORE, NOW)).toBe("~09:00 Sunday · GMT+8");
  });

  it("falls back to a date once a weekday name stops being unambiguous", () => {
    const at = new Date("2026-08-12T01:00:00Z"); // 09:00, twelve days out

    expect(formatNextFiring(at, SINGAPORE, NOW)).toBe("~09:00 12 Aug · GMT+8");
  });

  it("counts days in the schedule's zone, not the reader's", () => {
    // 2026-07-31T16:00Z is 1 Aug in Singapore but still 31 Jul in UTC. Reading
    // the day boundary in the wrong zone turns "tomorrow" into "today".
    const at = new Date("2026-07-31T16:00:00Z");

    expect(formatNextFiring(at, "UTC", NOW)).toContain("today");
    expect(formatNextFiring(at, SINGAPORE, NOW)).toContain("tomorrow");
  });
});
