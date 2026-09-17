import { describe, expect, it } from "vitest";
import {
  buildCreateRoutineBody,
  buildCreateTriggerBody,
  buildUpdateRoutineBody,
  normalizeRoutine,
  normalizeRoutineList,
  normalizeRoutineRun,
  normalizeRoutineRunList,
  normalizeRoutineTrigger,
  normalizeRoutineTriggerList,
} from "./office-routine-normalize";

const ROUTINE_NAME = "Daily sweep";
const LAST_RUN_AT = "2026-09-16T00:00:00Z";
const CREATED_AT = "2026-09-01T00:00:00Z";
const UPDATED_AT = "2026-09-02T00:00:00Z";
const RUN_TIMESTAMP = "2026-09-17T00:00:00Z";

describe("normalizeRoutine", () => {
  it("maps the backend routine wire shape to the web model", () => {
    expect(
      normalizeRoutine({
        id: "r1",
        workspace_id: "w1",
        name: ROUTINE_NAME,
        description: "runs every day",
        task_template: '{"title":"Sweep"}',
        assignee_agent_profile_id: "agent-1",
        status: "active",
        concurrency_policy: "coalesce_if_active",
        catch_up_policy: "skip_if_active",
        catch_up_max: 10,
        variables: '{"env":"prod"}',
        last_run_at: LAST_RUN_AT,
        created_at: CREATED_AT,
        updated_at: UPDATED_AT,
      }),
    ).toEqual({
      id: "r1",
      workspaceId: "w1",
      name: ROUTINE_NAME,
      description: "runs every day",
      taskTemplate: { title: "Sweep" },
      assigneeAgentProfileId: "agent-1",
      status: "active",
      concurrencyPolicy: "coalesce_if_active",
      catchUpPolicy: "skip_if_active",
      catchUpMax: 10,
      variables: { env: "prod" },
      lastRunAt: LAST_RUN_AT,
      createdAt: CREATED_AT,
      updatedAt: UPDATED_AT,
    });
  });

  it("produces empty string for required strings and undefined for optional fields when absent", () => {
    const routine = normalizeRoutine({ id: "r1" });
    expect(routine.name).toBe("");
    expect(routine.status).toBe("");
    expect(routine.concurrencyPolicy).toBe("");
    expect(routine.description).toBeUndefined();
    expect(routine.assigneeAgentProfileId).toBeUndefined();
    expect(routine.catchUpPolicy).toBeUndefined();
    expect(routine.catchUpMax).toBeUndefined();
    expect(routine.variables).toBeUndefined();
    expect(routine.lastRunAt).toBeUndefined();
  });

  it("produces an empty object for task_template when absent, null, or malformed", () => {
    expect(normalizeRoutine({}).taskTemplate).toEqual({});
    expect(normalizeRoutine({ task_template: null }).taskTemplate).toEqual({});
    expect(normalizeRoutine({ task_template: "" }).taskTemplate).toEqual({});
    expect(normalizeRoutine({ task_template: "not json" }).taskTemplate).toEqual({});
    expect(normalizeRoutine({ task_template: "[1,2]" }).taskTemplate).toEqual({});
    expect(normalizeRoutine({ task_template: "3" }).taskTemplate).toEqual({});
  });

  it("never clamps a catch_up_max the server sent, even below 1", () => {
    expect(normalizeRoutine({ catch_up_max: 0 }).catchUpMax).toBe(0);
    expect(normalizeRoutine({ catch_up_max: -5 }).catchUpMax).toBe(-5);
  });

  it("does not substitute a policy value the server did not send", () => {
    const routine = normalizeRoutine({ concurrency_policy: undefined, catch_up_policy: undefined });
    expect(routine.concurrencyPolicy).toBe("");
    expect(routine.catchUpPolicy).toBeUndefined();
  });

  it("produces an empty object for a present-but-malformed variables string, undefined only when absent", () => {
    expect(normalizeRoutine({ variables: "not json" }).variables).toEqual({});
    expect(normalizeRoutine({ variables: "" }).variables).toEqual({});
    expect(normalizeRoutine({}).variables).toBeUndefined();
    expect(normalizeRoutine({ variables: null }).variables).toBeUndefined();
  });
});

describe("normalizeRoutineTrigger", () => {
  it("maps the backend trigger wire shape to the web model, excluding secret", () => {
    expect(
      normalizeRoutineTrigger({
        id: "t1",
        routine_id: "r1",
        kind: "cron",
        cron_expression: "0 * * * *",
        timezone: "UTC",
        public_id: "pub-1",
        signing_mode: "hmac",
        secret: "super-secret",
        next_run_at: "2026-09-17T01:00:00Z",
        last_fired_at: "2026-09-16T01:00:00Z",
        enabled: true,
        created_at: CREATED_AT,
        updated_at: UPDATED_AT,
      }),
    ).toEqual({
      id: "t1",
      routineId: "r1",
      kind: "cron",
      cronExpression: "0 * * * *",
      timezone: "UTC",
      publicId: "pub-1",
      signingMode: "hmac",
      nextRunAt: "2026-09-17T01:00:00Z",
      lastFiredAt: "2026-09-16T01:00:00Z",
      enabled: true,
      createdAt: CREATED_AT,
      updatedAt: UPDATED_AT,
    });
  });

  it("defaults enabled to false and leaves optional fields undefined when absent", () => {
    const trigger = normalizeRoutineTrigger({ id: "t1" });
    expect(trigger.enabled).toBe(false);
    expect(trigger.cronExpression).toBeUndefined();
    expect(trigger.timezone).toBeUndefined();
  });
});

describe("normalizeRoutineRun", () => {
  it("maps the backend run wire shape to the web model", () => {
    expect(
      normalizeRoutineRun({
        id: "run1",
        routine_id: "r1",
        trigger_id: "t1",
        source: "cron",
        status: "done",
        trigger_payload: "{}",
        linked_task_id: "task-1",
        coalesced_into_run_id: "run0",
        dispatch_fingerprint: "fp-1",
        catch_up_missed_ticks: 3,
        catch_up_first_missed_at: LAST_RUN_AT,
        catch_up_truncated: true,
        started_at: RUN_TIMESTAMP,
        completed_at: "2026-09-17T00:01:00Z",
        created_at: RUN_TIMESTAMP,
      }),
    ).toEqual({
      id: "run1",
      routineId: "r1",
      triggerId: "t1",
      source: "cron",
      status: "done",
      triggerPayload: "{}",
      linkedTaskId: "task-1",
      coalescedIntoRunId: "run0",
      dispatchFingerprint: "fp-1",
      catchUpMissedTicks: 3,
      catchUpFirstMissedAt: LAST_RUN_AT,
      catchUpTruncated: true,
      startedAt: RUN_TIMESTAMP,
      completedAt: "2026-09-17T00:01:00Z",
      createdAt: RUN_TIMESTAMP,
    });
  });

  it("leaves catch_up_missed_ticks undefined rather than 0 when absent", () => {
    const run = normalizeRoutineRun({ id: "run1" });
    expect(run.catchUpMissedTicks).toBeUndefined();
    expect(run.catchUpTruncated).toBeUndefined();
  });
});

describe("list normalizers", () => {
  it("drop non-object elements and preserve order", () => {
    const raw = [{ id: "a" }, null, "garbage", 42, { id: "b" }];
    expect(normalizeRoutineList(raw).map((r) => r.id)).toEqual(["a", "b"]);
    expect(normalizeRoutineTriggerList(raw).map((r) => r.id)).toEqual(["a", "b"]);
    expect(normalizeRoutineRunList(raw).map((r) => r.id)).toEqual(["a", "b"]);
  });
});

describe("buildCreateRoutineBody", () => {
  it("sends only the supplied fields, JSON-encoding taskTemplate and variables", () => {
    expect(
      buildCreateRoutineBody({
        name: ROUTINE_NAME,
        taskTemplate: { title: "Sweep" },
      }),
    ).toEqual({
      name: ROUTINE_NAME,
      task_template: '{"title":"Sweep"}',
    });
  });

  it("omits concurrency_policy and catch_up_policy when not supplied", () => {
    const body = buildCreateRoutineBody({ name: "n" });
    expect(body).not.toHaveProperty("concurrency_policy");
    expect(body).not.toHaveProperty("catch_up_policy");
  });

  it("omits a catch_up_max below 1 rather than sending it", () => {
    expect(buildCreateRoutineBody({ name: "n", catchUpMax: 0 })).not.toHaveProperty("catch_up_max");
    expect(buildCreateRoutineBody({ name: "n", catchUpMax: -3 })).not.toHaveProperty(
      "catch_up_max",
    );
    expect(buildCreateRoutineBody({ name: "n", catchUpMax: 10 })).toHaveProperty(
      "catch_up_max",
      10,
    );
  });

  it("does not send workspace_id or status", () => {
    const body = buildCreateRoutineBody({ name: "n" });
    expect(body).not.toHaveProperty("workspace_id");
    expect(body).not.toHaveProperty("status");
  });
});

describe("buildUpdateRoutineBody", () => {
  it("omits a key the caller did not supply", () => {
    const body = buildUpdateRoutineBody({ name: "renamed" });
    expect(body).toEqual({ name: "renamed" });
  });

  it("transmits an empty string for name, description, taskTemplate, assigneeAgentProfileId, and variables to clear them", () => {
    const body = buildUpdateRoutineBody({
      name: "",
      description: "",
      taskTemplate: "",
      assigneeAgentProfileId: "",
      variables: "",
    });
    expect(body).toEqual({
      name: "",
      description: "",
      task_template: "",
      assignee_agent_profile_id: "",
      variables: "",
    });
  });

  it("never sends an empty status", () => {
    expect(buildUpdateRoutineBody({ status: "" })).not.toHaveProperty("status");
    expect(buildUpdateRoutineBody({})).not.toHaveProperty("status");
    expect(buildUpdateRoutineBody({ status: "paused" })).toHaveProperty("status", "paused");
  });

  it("omits an empty concurrencyPolicy or catchUpPolicy rather than clearing it", () => {
    const body = buildUpdateRoutineBody({ concurrencyPolicy: "", catchUpPolicy: "" });
    expect(body).not.toHaveProperty("concurrency_policy");
    expect(body).not.toHaveProperty("catch_up_policy");
  });

  it("omits a catch_up_max below 1 rather than sending it", () => {
    expect(buildUpdateRoutineBody({ catchUpMax: 0 })).not.toHaveProperty("catch_up_max");
    expect(buildUpdateRoutineBody({ catchUpMax: 25 })).toHaveProperty("catch_up_max", 25);
  });

  it("sends only the fields status admits, plus the general set", () => {
    const body = buildUpdateRoutineBody({
      name: "n",
      description: "d",
      taskTemplate: { a: 1 },
      assigneeAgentProfileId: "agent-1",
      status: "active",
      concurrencyPolicy: "coalesce_if_active",
      catchUpPolicy: "skip_if_active",
      catchUpMax: 10,
      variables: { env: "prod" },
    });
    expect(Object.keys(body).sort()).toEqual(
      [
        "name",
        "description",
        "task_template",
        "assignee_agent_profile_id",
        "status",
        "concurrency_policy",
        "catch_up_policy",
        "catch_up_max",
        "variables",
      ].sort(),
    );
  });
});

describe("buildCreateTriggerBody", () => {
  it("sends kind and the supplied optional fields under their wire keys", () => {
    expect(
      buildCreateTriggerBody({
        kind: "cron",
        cronExpression: "0 * * * *",
        timezone: "UTC",
      }),
    ).toEqual({
      kind: "cron",
      cron_expression: "0 * * * *",
      timezone: "UTC",
    });
  });

  it("trims the cron expression before sending it", () => {
    expect(buildCreateTriggerBody({ kind: "cron", cronExpression: "  0 * * * *  " })).toEqual({
      kind: "cron",
      cron_expression: "0 * * * *",
    });
  });

  it("omits fields the caller did not supply", () => {
    expect(buildCreateTriggerBody({ kind: "webhook" })).toEqual({ kind: "webhook" });
  });
});
