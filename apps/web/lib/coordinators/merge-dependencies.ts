import type { DepRef } from "@/lib/api/domains/coordinator-merge-api";

/** The identity of a reference for duplicate detection. */
export function refKey(ref: DepRef): string {
  return "task" in ref ? `task:${ref.task}` : `change:${ref.change}`;
}

export function refValue(ref: DepRef): string {
  return "task" in ref ? ref.task : ref.change;
}

/** A change key contains `|`; anything else is a task ID. Null for empty input. */
export function parseRefInput(input: string): DepRef | null {
  const value = input.trim();
  if (!value) return null;
  return value.includes("|") ? { change: value } : { task: value };
}

export type AddRefResult =
  | { ok: true; refs: DepRef[] }
  | { ok: false; reason: "empty" | "duplicate" | "self" };

/** Appends a reference unless it is empty, already listed or the card itself. */
export function addRef(refs: DepRef[], input: string, ownTaskId: string): AddRefResult {
  const ref = parseRefInput(input);
  if (!ref) return { ok: false, reason: "empty" };
  if ("task" in ref && ref.task === ownTaskId) return { ok: false, reason: "self" };
  if (refs.some((r) => refKey(r) === refKey(ref))) return { ok: false, reason: "duplicate" };
  return { ok: true, refs: [...refs, ref] };
}

export function removeRef(refs: DepRef[], ref: DepRef): DepRef[] {
  return refs.filter((r) => refKey(r) !== refKey(ref));
}

export function sameRefs(a: DepRef[], b: DepRef[]): boolean {
  if (a.length !== b.length) return false;
  const keys = new Set(a.map(refKey));
  return b.every((r) => keys.has(refKey(r)));
}

const DEP_ERROR_KEYS: Record<string, string> = {
  dependency_duplicate: "coordinator:mergeDepDuplicate",
  dependency_self: "coordinator:mergeDepSelf",
  dependency_invalid: "coordinator:mergeDepInvalid",
  dependency_unknown: "coordinator:mergeDepUnknown",
  revision_conflict: "coordinator:mergeDepConflict",
  revision_required: "coordinator:mergeDepConflict",
};

/** The i18n key of a refused dependency save; a generic failure for any other code. */
export function depErrorKey(code: string | null): string {
  return (code && DEP_ERROR_KEYS[code]) || "coordinator:mergeDepSaveFailed";
}
