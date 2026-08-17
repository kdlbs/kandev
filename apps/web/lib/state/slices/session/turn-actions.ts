import type { StateCreator } from "zustand";
import type { Draft } from "immer";
import type { SessionSlice } from "./types";
import type { Turn, TaskSession } from "@/lib/types/http";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

type TurnReconciliationDraft = Pick<Draft<SessionSlice>, "turns" | "taskSessions">;

// Strict wire format: full RFC3339 with explicit offset/UTC marker. JS
// Date.parse is permissive (e.g. "0" parses as 2000-01-01, partial dates as
// midnight, timezone-less values in the local zone), so shape validation must
// come first or malformed rows would win freshness comparisons.
const RFC3339_PATTERN =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(Z|[+-](\d{2}):(\d{2}))$/;

const DAYS_IN_MONTH = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

/** Whether the year is a leap year (proleptic Gregorian calendar). */
function isLeapYear(year: number): boolean {
  return (year % 4 === 0 && year % 100 !== 0) || year % 400 === 0;
}

/** Whether the day exists in the month (leap-year aware). */
function validDayForMonth(year: number, month: number, day: number): boolean {
  if (day < 1) return false;
  const maxDay = month === 2 && isLeapYear(year) ? 29 : DAYS_IN_MONTH[month - 1];
  return day <= maxDay;
}

/**
 * Returns the UTC-offset delta in SECONDS for a matched zone, or null when
 * the offset components are out of range.
 */
function parseOffsetSeconds(
  zone: string,
  offsetHourText: string | undefined,
  offsetMinuteText: string | undefined,
): number | null {
  if (zone === "Z") return 0;
  const hour = Number(offsetHourText);
  const minute = Number(offsetMinuteText);
  if (hour > 23 || minute > 59) return null;
  return (zone.startsWith("-") ? -1 : 1) * (hour * 60 + minute) * 60;
}

/**
 * Whether `incoming` is not newer than `existing`. A malformed incoming
 * timestamp counts as stale (never newer).
 */
function isNotNewerThan(incoming: string | undefined, existing: string | undefined): boolean {
  const incomingTs = parseTurnTimestamp(incoming);
  if (incomingTs === null) return true;
  const existingTs = parseTurnTimestamp(existing);
  if (existingTs === null) return false;
  return incomingTs <= existingTs;
}

/** Session states that settle a turn: no agent work is in progress. */
const SETTLED_SESSION_STATES = new Set<string>([
  "IDLE",
  "WAITING_FOR_INPUT",
  "COMPLETED",
  "FAILED",
  "CANCELLED",
]);

/**
 * Parsed settled boundary for a session, or null when none is recorded.
 */
function settledBoundary(draft: TurnReconciliationDraft, sessionId: string): bigint | null {
  return parseTurnTimestamp(draft.turns.settledBoundaryBySession[sessionId]);
}

/**
 * Whether a turn that STARTED at `startedAt` is on or before the session's
 * settled boundary — i.e. can never become active again (delayed WS start,
 * stale hydration, or force-merged snapshot naming it are all rejected).
 */
function isAtOrBeforeBoundary(
  draft: TurnReconciliationDraft,
  sessionId: string,
  startedAt: string | undefined,
): boolean {
  const boundary = settledBoundary(draft, sessionId);
  if (boundary === null) return false;
  const started = parseTurnTimestamp(startedAt);
  if (started === null) return false;
  return started <= boundary;
}

/** Whether the session state is settled — no agent work in progress. */
export function isSettledSessionState(state: string): boolean {
  return SETTLED_SESSION_STATES.has(state);
}

/**
 * Records the settled boundary for every settled session in `sessions` into
 * `boundaryBySession`, monotonically: a valid, strictly-newer candidate wins;
 * existing boundaries and their precision are preserved otherwise, and a
 * session without a parseable `updated_at` leaves its entry as it was. The
 * ONE definition of the seeding invariant — shared by the production
 * StateHydrator path and the SSR/boot merge path so the two can never drift
 * (a later fix applied to one would otherwise silently leave the other
 * accepting a stale delayed WS start).
 */
export function seedSettledSessionBoundaries(
  boundaryBySession: Record<string, string>,
  sessions: Iterable<TaskSession | undefined>,
): void {
  for (const session of sessions) {
    if (!session || !isSettledSessionState(session.state)) continue;
    const candidate = parseTurnTimestamp(session.updated_at);
    if (candidate === null) continue;
    const current = parseTurnTimestamp(boundaryBySession[session.id]);
    if (current === null || candidate > current) {
      boundaryBySession[session.id] = session.updated_at;
    }
  }
}

/**
 * Returns the id of the latest incomplete turn in the list, or null when
 * every turn is completed. Compares via the strict timestamp parser so
 * fractional (RFC3339Nano) and whole-second (RFC3339) started_at values
 * order correctly.
 */
export function latestIncompleteTurnId(turns: Turn[]): string | null {
  let latestId: string | null = null;
  let latestStarted: bigint | null = null;
  for (const turn of turns) {
    if (turn.completed_at) continue;
    const started = parseTurnTimestamp(turn.started_at);
    if (started === null) continue;
    if (latestStarted === null || started > latestStarted) {
      latestId = turn.id;
      latestStarted = started;
    }
  }
  return latestId;
}

/**
 * Establishes (or clears) the active-turn marker after a full REST hydration,
 * applying the same settled-session rule as reconcileActiveTurnForIdleSession
 * and rejecting hydrations that started before an authoritative clear.
 */
function applyActiveTurnReconciliation(
  draft: TurnReconciliationDraft,
  sessionId: string,
  hydrationEpoch: number,
): void {
  if ((draft.turns.reconcileEpochBySession[sessionId] ?? 0) !== hydrationEpoch) return;
  // Turns started at/before the settled boundary can never be active again —
  // exclude them all (not just the latest orphan), so older delayed starts
  // stay blocked too.
  const turns = (draft.turns.bySession[sessionId] ?? []).filter(
    (turn) => !isAtOrBeforeBoundary(draft, sessionId, turn.started_at),
  );
  const latestId = latestIncompleteTurnId(turns);
  if (latestId === null) {
    draft.turns.activeBySession[sessionId] = null;
    return;
  }
  // Mirror reconcileActiveTurnForIdleSession: when the session is currently
  // settled and its snapshot is at least as new as the candidate turn's
  // start, the candidate is an orphan — no active turn (files-panel source
  // gating reads this marker). The settled session update records the
  // boundary, so this is a fallback for sessions that settled without a
  // boundary (e.g. no session update crossed the client yet).
  const latestTurn = turns.find((turn) => turn.id === latestId);
  const session = draft.taskSessions.items[sessionId];
  if (session && latestTurn && isSettledSessionState(session.state)) {
    const sessionUpdatedAt = parseTurnTimestamp(session.updated_at);
    const turnStartedAt = parseTurnTimestamp(latestTurn.started_at);
    if (sessionUpdatedAt !== null && turnStartedAt !== null && turnStartedAt <= sessionUpdatedAt) {
      draft.turns.activeBySession[sessionId] = null;
      return;
    }
  }
  draft.turns.activeBySession[sessionId] = latestId;
}

/**
 * Merges a complete REST turn snapshot into an existing session list.
 *
 * The list is updated in one caller-owned Immer transaction. A map makes the
 * reconciliation linear in the number of rows, and the freshness predicate
 * prevents a delayed snapshot from rolling back a live WebSocket update.
 */
export function mergeTurnRows(target: Draft<Turn[]>, incoming: Turn[]): void {
  const existingById = new Map(target.map((turn) => [turn.id, turn]));
  for (const turn of incoming) {
    const existing = existingById.get(turn.id);
    if (!existing) {
      target.push(turn);
      existingById.set(turn.id, target[target.length - 1]);
      continue;
    }
    if (!shouldApplyTurnUpdate(existing as Turn, turn)) continue;
    Object.assign(existing, turn, { completed_at: existing.completed_at ?? turn.completed_at });
  }
}

/** Applies active-turn reconciliation to a draft owned by a hydration path. */
export function reconcileActiveTurnAfterHydrationDraft(
  draft: TurnReconciliationDraft,
  sessionId: string,
  hydrationEpoch: number,
): void {
  applyActiveTurnReconciliation(draft, sessionId, hydrationEpoch);
}

/**
 * Parses a turn `updated_at` into a comparable epoch in nanoseconds (BigInt).
 * Missing, empty, malformed, or non-RFC3339 values map to `null` (stale), so
 * they can never clobber a row with a valid timestamp. Calendar/time
 * components are validated explicitly because Date.parse NORMALIZES
 * semantically invalid values (e.g. `2026-02-30T10:00:00Z` becomes Mar 2)
 * instead of rejecting them. BigInt nanoseconds preserve Go RFC3339Nano
 * payloads exactly — float64 ms epochs have a ~244ns ULP around 2026, which
 * would collapse legitimate sub-100ns differences and misorder rows.
 */
export function parseTurnTimestamp(value: string | undefined): bigint | null {
  if (!value) return null;
  const match = RFC3339_PATTERN.exec(value);
  if (!match) return null;
  const year = Number(match[1]);
  const month = Number(match[2]);
  if (month < 1 || month > 12) return null;
  if (!validDayForMonth(year, month, Number(match[3]))) return null;
  if (Number(match[4]) > 23 || Number(match[5]) > 59 || Number(match[6]) > 59) {
    return null;
  }
  const offsetSeconds = parseOffsetSeconds(match[8], match[9], match[10]);
  if (offsetSeconds === null) return null;
  // RFC3339Nano caps the fraction at 9 digits; longer fractions would
  // overflow into the seconds component (padEnd does not cap).
  if (match[7] !== undefined && match[7].length > 9) return null;
  // setUTCFullYear handles years 0-99 correctly (Date.UTC maps them to
  // 1900+year); the components are validated above, so no normalization can
  // occur here.
  const utc = new Date(0);
  utc.setUTCFullYear(year, month - 1, Number(match[3]));
  utc.setUTCHours(Number(match[4]), Number(match[5]), Number(match[6]), 0);
  const wholeSeconds = BigInt(Math.floor(utc.getTime() / 1000)) - BigInt(offsetSeconds);
  const fractionNs = BigInt((match[7] ?? "").padEnd(9, "0"));
  return wholeSeconds * BigInt(1_000_000_000) + fractionNs;
}

/**
 * Decides whether an incoming turn row should replace the store's existing
 * row for the same turn. Completion state takes precedence over timestamps:
 * WS rows carry RFC3339Nano fractions while the REST DTO truncates to whole
 * seconds, so a completion within the same second can look equal or older —
 * but a completed row is always more advanced than an incomplete one, and an
 * existing completion must never be rolled back. Only when the completion
 * states agree do we fall back to `updated_at` freshness. This guards every
 * write path (WS events, REST hydration) against stale rows clobbering newer
 * live data.
 */
export function shouldApplyTurnUpdate(existing: Turn, incoming: Turn): boolean {
  const incomingCompleted = !!incoming.completed_at;
  const existingCompleted = !!existing.completed_at;
  if (incomingCompleted && !existingCompleted) return true;
  if (existingCompleted && !incomingCompleted) return false;
  const incomingTs = parseTurnTimestamp(incoming.updated_at);
  const existingTs = parseTurnTimestamp(existing.updated_at);
  if (incomingTs === null) return false;
  if (existingTs === null) return true;
  return incomingTs > existingTs;
}

/** Store action: merges a complete REST turn snapshot and reconciles its marker atomically. */
function mergeTurnsSnapshotAction(set: ImmerSet) {
  return (sessionId: string, turns: Turn[], hydrationEpoch: number) =>
    set((draft) => {
      if (!draft.taskSessions.items[sessionId]) return;
      const target = (draft.turns.bySession[sessionId] ??= []);
      mergeTurnRows(target, turns);
      applyActiveTurnReconciliation(draft, sessionId, hydrationEpoch);
      draft.turns.loadedBySession[sessionId] = true;
    });
}

/** Builds the turn store actions (upsert, completion, markers, snapshots). */
export function buildTurnActions(set: ImmerSet) {
  return {
    /**
     * Upserts a turn row, rejecting stale/out-of-order updates via
     * shouldApplyTurnUpdate and preserving an existing completion.
     */
    addTurn: (turn: Parameters<SessionSlice["addTurn"]>[0]) =>
      set((draft) => {
        const sessionId = turn.session_id;
        if (!draft.turns.bySession[sessionId]) draft.turns.bySession[sessionId] = [];
        const existing = draft.turns.bySession[sessionId].find((item) => item.id === turn.id);
        if (!existing) {
          draft.turns.bySession[sessionId].push(turn);
          return;
        }
        if (!shouldApplyTurnUpdate(existing, turn)) return;
        // Preserve an existing completed_at: WS arrivals carry nanosecond
        // precision (Go RFC3339Nano) while the REST DTO truncates to whole
        // seconds. When WS arrives first the high-precision value is kept and
        // a later REST row must not replace it with a truncated one. When
        // REST arrives first, a later WS nanosecond value is lost
        // (completed_at stays whole-second) — acceptable, because the field
        // only gates presence checks; note the third argument is evaluated
        // BEFORE the mutation, so this intentionally captures the pre-merge
        // value.
        Object.assign(existing, turn, { completed_at: existing.completed_at ?? turn.completed_at });
      }),
    /** Merges a complete turn snapshot and reconciles its marker atomically. */
    mergeTurnsSnapshot: mergeTurnsSnapshotAction(set),
    completeTurn: (
      sessionId: Parameters<SessionSlice["completeTurn"]>[0],
      turnId: Parameters<SessionSlice["completeTurn"]>[1],
      completedAt: Parameters<SessionSlice["completeTurn"]>[2],
      metadata: Parameters<SessionSlice["completeTurn"]>[3],
      updatedAt: Parameters<SessionSlice["completeTurn"]>[4],
    ) =>
      set((draft) => {
        const turn = draft.turns.bySession[sessionId]?.find((item) => item.id === turnId);
        if (turn) {
          // addTurn already applied the completed payload; only apply the
          // completion fields here when the event is not provably stale.
          // On an already-completed row, a missing, malformed, or
          // equal/older `updated_at` is stale — the same null policy as
          // isNotNewerThan, with an absent incoming timestamp treated as
          // stale too (the event type always carries updated_at).
          const stale =
            turn.completed_at !== undefined &&
            (updatedAt === undefined || isNotNewerThan(updatedAt, turn.updated_at));
          if (!stale) {
            turn.completed_at = completedAt;
            if (metadata) turn.metadata = metadata;
          }
        }
        // Clear unconditionally: even when the completion fields were stale
        // and the data block above was skipped, the active marker must be
        // retired — a completed turn can never be active. Decoupled from the
        // staleness check on purpose; the two blocks are not a missed guard.
        if (draft.turns.activeBySession[sessionId] === turnId) {
          draft.turns.activeBySession[sessionId] = null;
        }
      }),
    /** Records that the session's full persisted turn history is in the store. */
    markTurnsLoaded: (sessionId: string) =>
      set((draft) => {
        draft.turns.loadedBySession[sessionId] = true;
      }),
    /**
     * Reconciles the active-turn marker after a full REST hydration, applying
     * the settled-session rule and rejecting hydrations that started before
     * an authoritative clear (epoch mismatch).
     */
    reconcileActiveTurnAfterHydration: (sessionId: string, hydrationEpoch: number) =>
      set((draft) => {
        applyActiveTurnReconciliation(draft, sessionId, hydrationEpoch);
      }),
    setActiveTurn: (
      sessionId: Parameters<SessionSlice["setActiveTurn"]>[0],
      turnId: Parameters<SessionSlice["setActiveTurn"]>[1],
    ) =>
      set((draft) => {
        if (!turnId) {
          draft.turns.activeBySession[sessionId] = null;
          return;
        }
        const turns = draft.turns.bySession[sessionId] ?? [];
        const next = turns.find((turn) => turn.id === turnId);
        if (!next || next.completed_at) return;

        const currentId = draft.turns.activeBySession[sessionId];
        const current = turns.find((turn) => turn.id === currentId);
        if (!current || current.completed_at) {
          draft.turns.activeBySession[sessionId] = turnId;
          return;
        }
        if (current.id === turnId) return;

        // Compare with nanosecond precision: Date.parse collapses two starts
        // in the same millisecond, which would refuse to promote a genuinely
        // newer RFC3339Nano start (the WS guard already accepted it).
        const currentStartedAt = parseTurnTimestamp(current.started_at);
        const nextStartedAt = parseTurnTimestamp(next.started_at);
        if (
          currentStartedAt !== null &&
          nextStartedAt !== null &&
          nextStartedAt > currentStartedAt
        ) {
          draft.turns.activeBySession[sessionId] = turnId;
        }
      }),
    reconcileWorkspaceSourcesAdopted: (
      sessionIds: Parameters<SessionSlice["reconcileWorkspaceSourcesAdopted"]>[0],
      boundaryTimestamp?: Parameters<SessionSlice["reconcileWorkspaceSourcesAdopted"]>[1],
    ) =>
      set((draft) => {
        for (const sessionId of new Set(sessionIds)) {
          draft.turns.activeBySession[sessionId] = null;
          // Source adoption is an authoritative idle boundary: every turn
          // started before it can never be active again. The boundary must
          // come from the SERVER clock (the WS envelope timestamp that the
          // handler forwards), never from the client: a browser clock ahead
          // of the backend would record a future boundary and reject
          // legitimate new `session.turn.started` timestamps until server
          // time caught up, breaking turn-derived agent status and
          // file-source gating. When no server timestamp is available (e.g.
          // the optimistic REST submission, whose response carries none and
          // whose authoritative WS event follows), skip the boundary advance
          // entirely rather than fall back to the client clock — the marker
          // clear and epoch bump still apply, and the server-published
          // adoption event records the boundary on arrival.
          if (boundaryTimestamp) {
            const candidate = parseTurnTimestamp(boundaryTimestamp);
            const current = parseTurnTimestamp(draft.turns.settledBoundaryBySession[sessionId]);
            if (candidate !== null && (current === null || candidate > current)) {
              draft.turns.settledBoundaryBySession[sessionId] = boundaryTimestamp;
            }
          }
          // Bump the epoch so an in-flight REST hydration cannot resurrect
          // the marker either.
          draft.turns.reconcileEpochBySession[sessionId] =
            (draft.turns.reconcileEpochBySession[sessionId] ?? 0) + 1;
        }
      }),
  };
}
