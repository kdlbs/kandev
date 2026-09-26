import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  Routine,
  RoutineTrigger,
  RoutineRun,
  CreateRoutineInput,
  UpdateRoutinePatch,
  CreateTriggerInput,
} from "@/lib/state/slices/office/types";
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

const BASE = "/api/v1/office";

/** A body that is not a plain object has no envelope key and takes the same branch as a missing one. */
function unwrapEnvelopeList(body: unknown, key: string): unknown[] {
  if (!body || typeof body !== "object" || Array.isArray(body)) return [];
  const value = (body as Record<string, unknown>)[key];
  return Array.isArray(value) ? value : [];
}

// i18n-exempt: defensive diagnostic for a response shape the documented
// routine wire contract guarantees will not occur; not a message a normal
// user flow can trigger.
function unwrapEnvelopeResource(body: unknown, key: string): Record<string, unknown> {
  const container =
    body && typeof body === "object" && !Array.isArray(body)
      ? (body as Record<string, unknown>)
      : undefined;
  const value = container?.[key];
  const isObject =
    value !== undefined && value !== null && typeof value === "object" && !Array.isArray(value);
  const id = isObject ? (value as Record<string, unknown>).id : undefined;
  if (!isObject || typeof id !== "string" || id === "") {
    throw new Error(`office API: malformed "${key}" response`);
  }
  return value as Record<string, unknown>;
}

export async function listRoutines(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<{ routines: Routine[] }> {
  const res = await fetchJson<unknown>(`${BASE}/workspaces/${workspaceId}/routines`, options);
  return { routines: normalizeRoutineList(unwrapEnvelopeList(res, "routines")) };
}

export async function createRoutine(
  workspaceId: string,
  data: CreateRoutineInput,
  options?: ApiRequestOptions,
): Promise<Routine> {
  const res = await fetchJson<unknown>(`${BASE}/workspaces/${workspaceId}/routines`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(buildCreateRoutineBody(data)), ...options?.init },
  });
  return normalizeRoutine(unwrapEnvelopeResource(res, "routine"));
}

export async function getRoutine(id: string, options?: ApiRequestOptions): Promise<Routine> {
  const res = await fetchJson<unknown>(`${BASE}/routines/${id}`, options);
  return normalizeRoutine(unwrapEnvelopeResource(res, "routine"));
}

export async function updateRoutine(
  id: string,
  patch: UpdateRoutinePatch,
  options?: ApiRequestOptions,
): Promise<Routine> {
  const res = await fetchJson<unknown>(`${BASE}/routines/${id}`, {
    ...options,
    init: {
      method: "PATCH",
      body: JSON.stringify(buildUpdateRoutineBody(patch)),
      ...options?.init,
    },
  });
  return normalizeRoutine(unwrapEnvelopeResource(res, "routine"));
}

export function deleteRoutine(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/routines/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export async function runRoutine(
  id: string,
  variables?: Record<string, string>,
  options?: ApiRequestOptions,
): Promise<RoutineRun> {
  const res = await fetchJson<unknown>(`${BASE}/routines/${id}/run`, {
    ...options,
    init: {
      method: "POST",
      body: variables ? JSON.stringify({ variables }) : undefined,
      ...options?.init,
    },
  });
  return normalizeRoutineRun(unwrapEnvelopeResource(res, "run"));
}

export async function listRoutineTriggers(
  routineId: string,
  options?: ApiRequestOptions,
): Promise<{ triggers: RoutineTrigger[] }> {
  const res = await fetchJson<unknown>(`${BASE}/routines/${routineId}/triggers`, options);
  return { triggers: normalizeRoutineTriggerList(unwrapEnvelopeList(res, "triggers")) };
}

export async function createRoutineTrigger(
  routineId: string,
  data: CreateTriggerInput,
  options?: ApiRequestOptions,
): Promise<RoutineTrigger> {
  const res = await fetchJson<unknown>(`${BASE}/routines/${routineId}/triggers`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(buildCreateTriggerBody(data)), ...options?.init },
  });
  return normalizeRoutineTrigger(unwrapEnvelopeResource(res, "trigger"));
}

export function deleteRoutineTrigger(triggerId: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/routine-triggers/${triggerId}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}

export async function listRoutineRuns(
  routineId: string,
  options?: ApiRequestOptions,
): Promise<{ runs: RoutineRun[] }> {
  const res = await fetchJson<unknown>(`${BASE}/routines/${routineId}/runs`, options);
  return { runs: normalizeRoutineRunList(unwrapEnvelopeList(res, "runs")) };
}

export async function listAllRoutineRuns(
  workspaceId: string,
  options?: ApiRequestOptions,
): Promise<{ runs: RoutineRun[] }> {
  const res = await fetchJson<unknown>(`${BASE}/workspaces/${workspaceId}/routine-runs`, options);
  return { runs: normalizeRoutineRunList(unwrapEnvelopeList(res, "runs")) };
}
