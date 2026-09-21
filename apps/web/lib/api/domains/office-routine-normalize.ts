import type {
  CreateRoutineInput,
  CreateTriggerInput,
  Routine,
  RoutineRun,
  RoutineTrigger,
  ScheduleState,
  UpdateRoutinePatch,
  UnarmedCronTrigger,
  UnarmedReason,
} from "@/lib/state/slices/office/types";

type WireRecord = Record<string, unknown>;

function asRecord(raw: unknown): WireRecord {
  return raw && typeof raw === "object" && !Array.isArray(raw) ? (raw as WireRecord) : {};
}

function str(record: WireRecord, key: string): string {
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function optStr(record: WireRecord, key: string): string | undefined {
  const value = record[key];
  return typeof value === "string" ? value : undefined;
}

function optNum(record: WireRecord, key: string): number | undefined {
  const value = record[key];
  return typeof value === "number" ? value : undefined;
}

function optBool(record: WireRecord, key: string): boolean | undefined {
  const value = record[key];
  return typeof value === "boolean" ? value : undefined;
}

const SCHEDULE_STATES = new Set<ScheduleState>([
  "armed",
  "trigger_invalid",
  "trigger_unscheduled",
  "trigger_disabled",
  "event_only",
  "unscheduled_manual_only",
  "unscheduled_no_trigger",
  "unknown",
]);

const UNARMED_REASONS = new Set<UnarmedReason>(["disabled", "not_schedulable", "stalled"]);

function normalizeScheduleState(value: unknown): ScheduleState | undefined {
  if (typeof value !== "string") return undefined;
  return SCHEDULE_STATES.has(value as ScheduleState) ? (value as ScheduleState) : undefined;
}

function normalizeUnarmedCronTriggers(value: unknown): UnarmedCronTrigger[] {
  if (!Array.isArray(value)) return [];
  return value.filter(isJSONObject).map((entry) => {
    const trigger = asRecord(entry);
    const reasons = Array.isArray(trigger.reasons)
      ? trigger.reasons.filter(
          (reason): reason is UnarmedReason =>
            typeof reason === "string" && UNARMED_REASONS.has(reason as UnarmedReason),
        )
      : [];
    return { triggerId: str(trigger, "trigger_id"), reasons };
  });
}

function isJSONObject(value: unknown): value is WireRecord {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/** Parses a required JSON-object-encoded field: absent, malformed, or non-object decodes to {}. */
function parseRequiredJSONObject(value: unknown): Record<string, unknown> {
  if (typeof value !== "string" || !value) return {};
  try {
    const parsed: unknown = JSON.parse(value);
    return isJSONObject(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

/** Parses an optional JSON-object-encoded field: absent/null is undefined; a present string follows AC-003.4. */
function parseOptionalJSONObject(value: unknown): Record<string, unknown> | undefined {
  if (value === undefined || value === null) return undefined;
  return parseRequiredJSONObject(value);
}

/** Serializes a write-side task_template/variables value: object -> JSON text, string passes through, undefined stays undefined. */
function stringifyJSONField(
  value: Record<string, unknown> | string | undefined,
): string | undefined {
  if (value === undefined) return undefined;
  return typeof value === "string" ? value : JSON.stringify(value);
}

function assignIfDefined(body: WireRecord, key: string, value: unknown): void {
  if (value !== undefined) body[key] = value;
}

export function normalizeRoutine(raw: unknown): Routine {
  const r = asRecord(raw);
  const routine: Routine = {
    id: str(r, "id"),
    workspaceId: str(r, "workspace_id"),
    name: str(r, "name"),
    description: optStr(r, "description"),
    taskTemplate: parseRequiredJSONObject(r.task_template),
    assigneeAgentProfileId: optStr(r, "assignee_agent_profile_id"),
    status: str(r, "status"),
    concurrencyPolicy: str(r, "concurrency_policy"),
    catchUpPolicy: optStr(r, "catch_up_policy"),
    catchUpMax: optNum(r, "catch_up_max"),
    variables: parseOptionalJSONObject(r.variables),
    lastRunAt: optStr(r, "last_run_at"),
    createdAt: str(r, "created_at"),
    updatedAt: str(r, "updated_at"),
  };
  const scheduleState = normalizeScheduleState(r.schedule_state);
  if (scheduleState !== undefined) routine.scheduleState = scheduleState;
  if (Array.isArray(r.unarmed_cron_triggers)) {
    routine.unarmedCronTriggers = normalizeUnarmedCronTriggers(r.unarmed_cron_triggers);
  }
  return routine;
}

export function normalizeRoutineTrigger(raw: unknown): RoutineTrigger {
  const r = asRecord(raw);
  return {
    id: str(r, "id"),
    routineId: str(r, "routine_id"),
    kind: str(r, "kind"),
    cronExpression: optStr(r, "cron_expression"),
    timezone: optStr(r, "timezone"),
    publicId: optStr(r, "public_id"),
    signingMode: optStr(r, "signing_mode"),
    nextRunAt: optStr(r, "next_run_at"),
    lastFiredAt: optStr(r, "last_fired_at"),
    enabled: Boolean(r.enabled === true),
    createdAt: str(r, "created_at"),
    updatedAt: str(r, "updated_at"),
  };
}

export function normalizeRoutineRun(raw: unknown): RoutineRun {
  const r = asRecord(raw);
  return {
    id: str(r, "id"),
    routineId: str(r, "routine_id"),
    triggerId: optStr(r, "trigger_id"),
    source: str(r, "source"),
    status: str(r, "status"),
    triggerPayload: optStr(r, "trigger_payload"),
    linkedTaskId: optStr(r, "linked_task_id"),
    coalescedIntoRunId: optStr(r, "coalesced_into_run_id"),
    dispatchFingerprint: optStr(r, "dispatch_fingerprint"),
    catchUpMissedTicks: optNum(r, "catch_up_missed_ticks"),
    catchUpFirstMissedAt: optStr(r, "catch_up_first_missed_at"),
    catchUpTruncated: optBool(r, "catch_up_truncated"),
    startedAt: optStr(r, "started_at"),
    completedAt: optStr(r, "completed_at"),
    createdAt: str(r, "created_at"),
  };
}

function normalizeList<T>(raw: unknown[], normalize: (raw: unknown) => T): T[] {
  return raw.filter(isJSONObject).map(normalize);
}

export function normalizeRoutineList(raw: unknown[]): Routine[] {
  return normalizeList(raw, normalizeRoutine);
}

export function normalizeRoutineTriggerList(raw: unknown[]): RoutineTrigger[] {
  return normalizeList(raw, normalizeRoutineTrigger);
}

export function normalizeRoutineRunList(raw: unknown[]): RoutineRun[] {
  return normalizeList(raw, normalizeRoutineRun);
}

export function buildCreateRoutineBody(input: CreateRoutineInput): WireRecord {
  const body: WireRecord = { name: input.name };
  assignIfDefined(body, "description", input.description);
  assignIfDefined(body, "task_template", stringifyJSONField(input.taskTemplate));
  assignIfDefined(body, "assignee_agent_profile_id", input.assigneeAgentProfileId);
  assignIfDefined(body, "concurrency_policy", input.concurrencyPolicy);
  assignIfDefined(body, "catch_up_policy", input.catchUpPolicy);
  if (input.catchUpMax !== undefined && input.catchUpMax >= 1) {
    body.catch_up_max = input.catchUpMax;
  }
  assignIfDefined(body, "variables", stringifyJSONField(input.variables));
  return body;
}

export function buildUpdateRoutineBody(patch: UpdateRoutinePatch): WireRecord {
  const body: WireRecord = {};
  assignIfDefined(body, "name", patch.name);
  assignIfDefined(body, "description", patch.description);
  assignIfDefined(body, "task_template", stringifyJSONField(patch.taskTemplate));
  assignIfDefined(body, "assignee_agent_profile_id", patch.assigneeAgentProfileId);
  if (patch.status) body.status = patch.status;
  assignIfDefined(body, "variables", stringifyJSONField(patch.variables));
  if (patch.concurrencyPolicy) body.concurrency_policy = patch.concurrencyPolicy;
  if (patch.catchUpPolicy) body.catch_up_policy = patch.catchUpPolicy;
  if (patch.catchUpMax !== undefined && patch.catchUpMax >= 1) {
    body.catch_up_max = patch.catchUpMax;
  }
  return body;
}

export function buildCreateTriggerBody(input: CreateTriggerInput): WireRecord {
  const body: WireRecord = { kind: input.kind };
  assignIfDefined(body, "cron_expression", input.cronExpression?.trim());
  assignIfDefined(body, "timezone", input.timezone);
  assignIfDefined(body, "public_id", input.publicId);
  assignIfDefined(body, "signing_mode", input.signingMode);
  assignIfDefined(body, "secret", input.secret);
  return body;
}
