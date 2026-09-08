/**
 * Canonical-value / in-flight-guard concurrency protocol for Office task
 * title + description editing (docs/specs/office-task-content-editing/spec.md,
 * section E). Framework-agnostic, module-level singleton state per task.
 *
 * Two structures are kept deliberately separate (see the task plan's R5-F4
 * record): `canonicalByTaskField` never loses a value when a field becomes
 * unguarded, while `guardByTaskField` entries are garbage-collected as soon
 * as a field has neither an open editor nor an unresolved write.
 */

export type SyncField = "title" | "description";
type CandidateKind = "refetch" | "write";

type CanonicalRecord = {
  value: string;
  updatedAtNs: bigint | null;
  sequence: number;
};

type GuardState = {
  editorOpen: boolean;
  pendingWrites: Set<number>;
};

const FIELDS: SyncField[] = ["title", "description"];

const canonicalByTaskField = new Map<string, Map<SyncField, CanonicalRecord>>();
const guardByTaskField = new Map<string, Map<SyncField, GuardState>>();
const sequenceByTask = new Map<string, number>();
const lastWriteIssueSequenceByTaskField = new Map<string, Map<SyncField, number>>();
const lastSuccessfulWriteSequenceByTaskField = new Map<string, Map<SyncField, number>>();
const listenersByTaskField = new Map<string, Map<SyncField, Set<(value: string) => void>>>();

function fieldMap<T>(store: Map<string, Map<SyncField, T>>, taskId: string): Map<SyncField, T> {
  let m = store.get(taskId);
  if (!m) {
    m = new Map();
    store.set(taskId, m);
  }
  return m;
}

/** Monotonic per-task counter; call at request-issue time (refetch or write). */
export function nextTaskSequence(taskId: string): number {
  const next = (sequenceByTask.get(taskId) ?? 0) + 1;
  sequenceByTask.set(taskId, next);
  return next;
}

function normalizeValue(raw: string | null | undefined): string {
  // AC-71: description normalises an absent value to "". Title's DTO
  // declares it required, but applying the same fallback to both is harmless.
  return raw ?? "";
}

const DATETIME_LAYOUT =
  /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

/**
 * Parses both `updated_at` layouts in play (AC-66): the space-separated,
 * offset-bearing SQLite bind layout, and RFC3339Nano. Returns nanoseconds
 * since the Unix epoch as a bigint, not `Date.parse`'s millisecond number —
 * `Date.parse` truncates any fractional-second precision past milliseconds,
 * which would silently defeat AC-60/62's strict ordering guarantee for two
 * candidates issued within the same millisecond (exactly what a refetch
 * racing a write's own response produces). The fractional-second digits are
 * parsed directly rather than handed to `Date.parse`, whose input must not
 * carry the space-separated form as-is since browsers disagree on how (or
 * whether) to interpret it.
 */
export function parseUpdatedAt(raw: string | null | undefined): bigint | null {
  if (!raw) return null;
  const match = DATETIME_LAYOUT.exec(raw.trim());
  if (!match) return null;
  const [, year, month, day, hour, minute, second, frac = "", offset] = match;
  const wholeSecondMs = Date.parse(`${year}-${month}-${day}T${hour}:${minute}:${second}${offset}`);
  if (Number.isNaN(wholeSecondMs)) return null;
  const nanoDigits = `${frac.slice(1)}000000000`.slice(0, 9);
  return BigInt(wholeSecondMs) * BigInt(1_000_000) + BigInt(nanoDigits);
}

export function isFieldGuarded(taskId: string, field: SyncField): boolean {
  const g = guardByTaskField.get(taskId)?.get(field);
  return !!g && (g.editorOpen || g.pendingWrites.size > 0);
}

export function getCanonicalValue(taskId: string, field: SyncField): string | undefined {
  return canonicalByTaskField.get(taskId)?.get(field)?.value;
}

function notifyField(taskId: string, field: SyncField): void {
  const value = getCanonicalValue(taskId, field);
  if (value === undefined) return;
  const listeners = listenersByTaskField.get(taskId)?.get(field);
  if (!listeners || listeners.size === 0) return;
  listeners.forEach((listener) => listener(value));
}

/**
 * Subscribes to a field's deferred-apply notifications (AC-27/AC-50). Fired
 * whenever a candidate is accepted while the field is unguarded, and whenever
 * the field transitions from guarded to unguarded. The listener receives the
 * current canonical value and is responsible for its own "if the two differ"
 * comparison against whatever it currently displays.
 */
export function subscribeField(
  taskId: string,
  field: SyncField,
  listener: (value: string) => void,
): () => void {
  const tm = fieldMap(listenersByTaskField, taskId);
  const set = tm.get(field) ?? new Set<(value: string) => void>();
  set.add(listener);
  tm.set(field, set);
  return () => {
    set.delete(listener);
  };
}

/**
 * Records a candidate canonical value for a field. Implements the seed rule
 * (AC-72), the parse-failure discard (AC-67), the unknown-instant-loses rule
 * (AC-73), the strict-ordering discard (AC-60/AC-62), the equal-instant
 * tiebreak by request-issue sequence (AC-63), and — for refetch candidates
 * only — the write-predates-refetch staleness discard (AC-52). Returns
 * whether the candidate was accepted. On acceptance, fires a deferred-apply
 * notification immediately if the field is currently unguarded; otherwise the
 * eventual guard release will notify once the guard clears (AC-50).
 */
type Candidate = {
  rawValue: string | null | undefined;
  updatedAtRaw: string | null | undefined;
  sequence: number;
  kind: CandidateKind;
};

function recordCandidate(taskId: string, field: SyncField, candidate: Candidate): boolean {
  const { rawValue, updatedAtRaw, sequence, kind } = candidate;
  const canonicalMap = fieldMap(canonicalByTaskField, taskId);
  const current = canonicalMap.get(field);
  const value = normalizeValue(rawValue);
  const candidateNs = parseUpdatedAt(updatedAtRaw);

  // AC-72: until a field has a recorded canonical value, this is a seed and
  // every other rule below (including AC-52's staleness check) is exempted —
  // there is nothing yet to be stale, out of order, or unparseable relative to.
  if (current && kind === "refetch") {
    const lastWriteSeq = lastWriteIssueSequenceByTaskField.get(taskId)?.get(field);
    if (lastWriteSeq !== undefined && sequence < lastWriteSeq) {
      return false; // AC-52
    }
  }

  if (current) {
    if (candidateNs === null) {
      return false; // AC-67
    }
    if (current.updatedAtNs !== null) {
      if (candidateNs < current.updatedAtNs) return false; // AC-60 / AC-62
      if (candidateNs === current.updatedAtNs && sequence <= current.sequence) return false; // AC-63 loser
    }
    // Either candidateNs > current.updatedAtNs, or candidateNs === current.updatedAtNs
    // with a higher sequence, or current.updatedAtNs was unparseable (AC-73).
  }

  canonicalMap.set(field, { value, updatedAtNs: candidateNs, sequence });
  if (!isFieldGuarded(taskId, field)) {
    notifyField(taskId, field);
  }
  return true;
}

/**
 * AC-59: seeds a field's canonical value on initial page load. The first-ever
 * call for a (taskId, field) is an unconditional AC-72 seed; a same-session
 * revisit's call goes through `recordCandidate`'s normal ordering rules like
 * any other candidate, so a fresh load can supersede a stale canonical value
 * left over from an earlier visit instead of being silently dropped.
 */
export function seedInitialCanonical(
  taskId: string,
  field: SyncField,
  value: string,
  updatedAtRaw: string | null | undefined,
): void {
  recordCandidate(taskId, field, {
    rawValue: value,
    updatedAtRaw,
    sequence: nextTaskSequence(taskId),
    kind: "refetch",
  });
}

/** AC-49: records a non-stale refetch's value as canonical for a field. */
export function recordRefetchCandidate(
  taskId: string,
  field: SyncField,
  rawValue: string | null | undefined,
  updatedAtRaw: string | null | undefined,
  sequence: number,
): boolean {
  return recordCandidate(taskId, field, { rawValue, updatedAtRaw, sequence, kind: "refetch" });
}

/**
 * AC-58: records a successful write's own response value as canonical.
 * Must be called before the caller releases the write's guard contribution
 * via `endWrite` (R5-F3: a failed write must never call this).
 */
export function recordWriteSuccess(
  taskId: string,
  field: SyncField,
  rawValue: string | null | undefined,
  updatedAtRaw: string | null | undefined,
  sequence: number,
): boolean {
  const fm = fieldMap(lastSuccessfulWriteSequenceByTaskField, taskId);
  const current = fm.get(field);
  if (current === undefined || sequence > current) {
    fm.set(field, sequence);
  }
  return recordCandidate(taskId, field, { rawValue, updatedAtRaw, sequence, kind: "write" });
}

/**
 * AC-44/AC-53: decides whether a failed write should restore the field's
 * canonical value. Must be called before `endWrite` removes this write's own
 * sequence from the in-flight set. Restore is suppressed when a *later*
 * (higher-sequence) commit to the same field has already succeeded (AC-44) or
 * is still unresolved (AC-53) — an earlier-sequenced commit in either state
 * does not suppress it (R5-F6).
 */
export function shouldRestoreAfterFailedWrite(
  taskId: string,
  field: SyncField,
  sequence: number,
): boolean {
  const lastSuccess = lastSuccessfulWriteSequenceByTaskField.get(taskId)?.get(field);
  if (lastSuccess !== undefined && lastSuccess > sequence) return false; // AC-44

  const pending = guardByTaskField.get(taskId)?.get(field)?.pendingWrites;
  if (pending) {
    for (const pendingSeq of pending) {
      if (pendingSeq > sequence) return false; // AC-53
    }
  }
  return true;
}

function releaseIfUnguarded(
  taskId: string,
  field: SyncField,
  gm: Map<SyncField, GuardState>,
  guard: GuardState,
): void {
  if (!guard.editorOpen && guard.pendingWrites.size === 0) {
    gm.delete(field);
    notifyField(taskId, field);
  }
}

/** AC-28/AC-38: an open editor is one of the two guard facts. */
export function openFieldEditor(taskId: string, field: SyncField): void {
  const gm = fieldMap(guardByTaskField, taskId);
  const guard = gm.get(field) ?? { editorOpen: false, pendingWrites: new Set<number>() };
  guard.editorOpen = true;
  gm.set(field, guard);
}

/** Closing the editor by any route (commit, cancel, blur) ends its guard contribution. */
export function closeFieldEditor(taskId: string, field: SyncField): void {
  const gm = guardByTaskField.get(taskId);
  const guard = gm?.get(field);
  if (!gm || !guard) return;
  guard.editorOpen = false;
  releaseIfUnguarded(taskId, field, gm, guard);
}

/**
 * Registers an in-flight write's guard contribution and records this write
 * as the field's most-recently-issued write for AC-52 staleness comparisons.
 */
export function beginWrite(taskId: string, field: SyncField, sequence: number): void {
  const gm = fieldMap(guardByTaskField, taskId);
  const guard = gm.get(field) ?? { editorOpen: false, pendingWrites: new Set<number>() };
  guard.pendingWrites.add(sequence);
  gm.set(field, guard);
  fieldMap(lastWriteIssueSequenceByTaskField, taskId).set(field, sequence);
}

/** AC-70: ends a write's guard contribution identically whether it succeeded or failed. */
export function endWrite(taskId: string, field: SyncField, sequence: number): void {
  const gm = guardByTaskField.get(taskId);
  const guard = gm?.get(field);
  if (!gm || !guard) return;
  guard.pendingWrites.delete(sequence);
  releaseIfUnguarded(taskId, field, gm, guard);
}

/**
 * AC-68: ends the open-editor contribution for both fields on unmount /
 * route change. Deliberately leaves `pendingWriteCount` untouched — a write
 * issued before unmount must still resolve and record (AC-69).
 */
export function endAllEditorsForTask(taskId: string): void {
  const gm = guardByTaskField.get(taskId);
  if (!gm) return;
  for (const field of FIELDS) {
    const guard = gm.get(field);
    if (guard?.editorOpen) {
      guard.editorOpen = false;
      releaseIfUnguarded(taskId, field, gm, guard);
    }
  }
}

/** Test-only: clears all module state between test cases. */
export function __resetOfficeTaskContentSyncForTests(): void {
  canonicalByTaskField.clear();
  guardByTaskField.clear();
  sequenceByTask.clear();
  lastWriteIssueSequenceByTaskField.clear();
  lastSuccessfulWriteSequenceByTaskField.clear();
  listenersByTaskField.clear();
}
