import type { TFunction } from "i18next";
import { formatCompactDuration, formatRelativeTime, formatTime } from "@/lib/i18n/formats";
import type { OverviewEvent } from "@/lib/types/control-overview";
const SESSION_STATE_LABEL_KEYS: Record<string, string> = {
  CREATED: "common:sessionStateCreated",
  STARTING: "common:sessionStateStarting",
  RUNNING: "common:sessionStateRunning",
  IDLE: "common:sessionStateIdle",
  WAITING_FOR_INPUT: "common:sessionStateWaitingForInput",
  COMPLETED: "common:sessionStateCompleted",
  FAILED: "common:sessionStateFailed",
  CANCELLED: "common:sessionStateCancelled",
};

const CIRCUIT_REASON_KEYS: Record<string, string> = {
  rate_limited: "office:overviewCircuitReasonRateLimited",
  quota_limited: "office:overviewCircuitReasonQuotaLimited",
  provider_unavailable: "office:overviewCircuitReasonProviderUnavailable",
  model_capacity: "office:overviewCircuitReasonModelCapacity",
};

/** What a blocked circuit's scope covers, in the operator's terms. */
const CIRCUIT_SCOPE_KEYS: Record<string, string> = {
  credential: "office:overviewCircuitScopeAccount",
  model: "office:overviewCircuitScopeModel",
  profile: "office:overviewCircuitScopeProfile",
  account_block: "office:overviewCircuitScopeProfile",
};

export function sessionStateLabel(t: TFunction, state: string | undefined): string {
  if (!state) return "";
  const key = SESSION_STATE_LABEL_KEYS[state];
  return key ? t(key) : state;
}

/**
 * Task states, phrased by the shared `common:taskState*` catalog the board and
 * the task lists already use, so a task state reads the same in a failure
 * follow-up as it does everywhere else. A state this catalog does not name keeps
 * its own code rather than being given invented wording: an unrecognised state is
 * reported as the code it is.
 */
const TASK_STATE_LABEL_KEYS: Record<string, string> = {
  TODO: "common:taskStateTodo",
  CREATED: "common:taskStateCreated",
  SCHEDULING: "common:taskStateScheduling",
  IN_PROGRESS: "common:taskStateInProgress",
  REVIEW: "common:taskStateReview",
  WAITING_FOR_INPUT: "common:taskStateWaitingForInput",
  BLOCKED: "common:taskStateBlocked",
  COMPLETED: "common:taskStateCompleted",
  FAILED: "common:taskStateFailed",
  CANCELLED: "common:taskStateCancelled",
  NOT_STARTED: "common:taskStateNotStarted",
};

export function taskStateLabel(t: TFunction, state: string | undefined): string {
  if (!state) return "";
  const key = TASK_STATE_LABEL_KEYS[state.toUpperCase()];
  return key ? t(key) : state;
}

export function durationFromMinutes(minutes: number): string {
  if (minutes < 60) return formatCompactDuration(minutes, "minute");
  if (minutes < 24 * 60) return formatCompactDuration(minutes / 60, "hour");
  return formatCompactDuration(minutes / (24 * 60), "day");
}

/** Elapsed time since an ISO timestamp as a compact duration. */
export function durationSince(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) return "";
  const at = new Date(iso).getTime();
  if (Number.isNaN(at)) return "";
  return durationFromMinutes(Math.max(0, Math.floor((now - at) / 60_000)));
}

/** "12 minutes ago" in the active locale, or "" when absent. */
export function relativeTime(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) return "";
  return formatRelativeTime(iso, now);
}

/**
 * When something happened, as a fixed clock reading: today's `HH:mm`, and
 * `MM-DD HH:mm` for anything earlier.
 *
 * A relative reading ("3 minutes ago") is only correct at the instant it was
 * written, so a list left open either restates it or needs a ticker to stay
 * truthful. The instant a thing happened does not move, so it is shown as the
 * clock time it was, with the relative reading kept for hover where a reader
 * asks "how long ago is that?". Durations are a different question and keep
 * using `durationSince`.
 *
 * The clock cycle and the date order are pinned rather than left to the locale.
 * A locale's own default gives "11:01 PM", and its date order gives "10/04" in
 * one locale and "04/10" in another, which is not the reading this column
 * promises. The 24-hour cycle comes from `Intl` so the digits stay locale-aware,
 * and the month-day order is written here because no option asks `Intl` for it.
 */
export function occurredTime(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) return "";
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "";
  const today = new Date(now);
  const sameDay =
    at.getFullYear() === today.getFullYear() &&
    at.getMonth() === today.getMonth() &&
    at.getDate() === today.getDate();
  const clock = formatTime(at, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  if (sameDay) return clock;
  const month = String(at.getMonth() + 1).padStart(2, "0");
  const day = String(at.getDate()).padStart(2, "0");
  return `${month}-${day} ${clock}`;
}

export function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

/**
 * The identity a blocked resource can name about itself. Both the
 * blocked-circuits card and a model-block event in the last-24-hours list carry
 * exactly these fields, so one rule phrases both and a circuit can never be
 * reported one way in one place and another way in the other.
 */
export type CircuitSubject = {
  resource_key?: string;
  scope_value?: string;
  profile_id?: string;
  profile_name?: string;
  model_name?: string;
};

/**
 * The identifier an unnamed circuit is reported by. The scope prefix names the
 * kind of resource, not which one, so it is dropped: a fingerprint's first
 * characters identify the resource, and the constant scope word does not.
 */
function circuitFallbackId(circuit: CircuitSubject): string {
  if (circuit.profile_id) return circuit.profile_id;
  const value = circuit.scope_value || circuit.resource_key || "";
  const binding = value.includes("|") ? value.slice(0, value.lastIndexOf("|")) : value;
  const withoutScope = binding.includes(":") ? binding.slice(binding.indexOf(":") + 1) : binding;
  return withoutScope || circuit.resource_key || "";
}

/**
 * The blocked circuit's subject: the profile name, qualified by the model when
 * the circuit covers one model. A circuit that names no profile keeps whatever
 * model it did name, and reports the profile as unidentifiable with a short id,
 * because the raw key is a fingerprint the operator cannot read.
 */
export function circuitTitle(t: TFunction, circuit: CircuitSubject): string {
  if (!circuit.profile_name) {
    const unknown = t("office:overviewCircuitUnknownName", {
      id: shortId(circuitFallbackId(circuit)),
    });
    return circuit.model_name ? `${unknown} · ${circuit.model_name}` : unknown;
  }
  return circuit.model_name
    ? `${circuit.profile_name} · ${circuit.model_name}`
    : circuit.profile_name;
}

/** A blocked circuit's reason in plain words, or the wire code when unphrased. */
export function circuitReason(t: TFunction, code: string | undefined): string {
  if (!code) return "";
  const key = CIRCUIT_REASON_KEYS[code];
  return key ? t(key) : code;
}

/** What a blocked circuit's scope covers, or "" for a scope with no wording. */
export function circuitScope(t: TFunction, scope: string): string {
  const key = CIRCUIT_SCOPE_KEYS[scope];
  return key ? t(key) : "";
}

/**
 * A model-block event's subject, or undefined when the row names no circuit. A
 * provider limit is already named by the provider it carries, so its title is
 * read as sent; a circuit carries the same fields the blocked-circuits card
 * does and is phrased by the same rule.
 */
export function modelBlockSubject(event: OverviewEvent): CircuitSubject | undefined {
  const isBlock = event.kind === "model_blocked" || event.kind === "model_unblocked";
  if (!isBlock || !event.scope) return undefined;
  return {
    scope_value: event.scope_value,
    profile_id: event.profile_id,
    profile_name: event.profile_name,
    model_name: event.model_name,
  };
}

/**
 * What a block covers and why it was blocked, in the blocked-circuits card's
 * wording. Empty when the row named neither, which is the case for a provider
 * limit: it has no scope and its own name already says what it is.
 */
export function blockFacts(t: TFunction, scope?: string, code?: string): string {
  return [circuitScope(t, scope ?? ""), circuitReason(t, code)].filter(Boolean).join(" · ");
}

// Provider usage is phrased by one rule, read here so the account card and the
// model card cannot drift apart on the same reading.

// A usage read that did not succeed reports one of two things: why the provider
// refused (agents:profileUsageReason*) or what shape the reading was in
// (office:overviewUsageState*). The two code sets do not overlap.
