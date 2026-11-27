import { parseStrictRfc3339Timestamp } from "@/lib/utils/strict-timestamp";
import type { RoutingCircuitWire } from "@/lib/api/domains/routing-circuits-api";

export const ROUTING_CIRCUIT_STATES = ["closed", "open", "half_open"] as const;
export type RoutingCircuitState = (typeof ROUTING_CIRCUIT_STATES)[number];

export type RoutingCircuit = Omit<RoutingCircuitWire, "state"> & {
  state: RoutingCircuitState;
};
export type RoutingCircuits = { circuits: RoutingCircuit[] };

/** The operator's word for each circuit state; the wire value is not copy. */
export const CIRCUIT_STATE_KEYS: Record<RoutingCircuitState, string> = {
  closed: "sidebar:controlCircuits.state.closed",
  open: "sidebar:controlCircuits.state.open",
  half_open: "sidebar:controlCircuits.state.halfOpen",
};

/**
 * What a circuit isolates. A scope this build does not name keeps its own
 * value, the way an unrecognised session state keeps its code.
 */
export const CIRCUIT_SCOPE_KEYS: Record<string, string> = {
  credential: "sidebar:controlCircuits.scope.credential",
  model: "sidebar:controlCircuits.scope.model",
  profile: "sidebar:controlCircuits.scope.profile",
  account_block: "sidebar:controlCircuits.scope.accountBlock",
};

/** Provider failure reasons the router records on a circuit. */
export const CIRCUIT_CODE_KEYS: Record<string, string> = {
  rate_limited: "sidebar:controlCircuits.code.rateLimited",
  quota_limited: "sidebar:controlCircuits.code.quotaLimited",
  provider_unavailable: "sidebar:controlCircuits.code.providerUnavailable",
  provider_overloaded: "sidebar:controlCircuits.code.providerOverloaded",
  model_capacity: "sidebar:controlCircuits.code.modelCapacity",
  model_unavailable: "sidebar:controlCircuits.code.modelUnavailable",
  auth_required: "sidebar:controlCircuits.code.authRequired",
  subscription_required: "sidebar:controlCircuits.code.subscriptionRequired",
  account_blocked: "sidebar:controlCircuits.code.accountBlocked",
  missing_credentials: "sidebar:controlCircuits.code.missingCredentials",
  provider_not_configured: "sidebar:controlCircuits.code.providerNotConfigured",
  network_unavailable: "sidebar:controlCircuits.code.networkUnavailable",
};

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function optionalTimestamp(value: unknown) {
  return (
    value === undefined ||
    (typeof value === "string" && parseStrictRfc3339Timestamp(value) !== null)
  );
}

function validCircuit(value: unknown): value is RoutingCircuitWire {
  return (
    record(value) &&
    typeof value.resource_key === "string" &&
    value.resource_key.length > 0 &&
    typeof value.scope === "string" &&
    typeof value.scope_value === "string" &&
    ROUTING_CIRCUIT_STATES.includes(value.state as RoutingCircuitState) &&
    (value.code === undefined || typeof value.code === "string") &&
    typeof value.failures === "number" &&
    Number.isInteger(value.failures) &&
    value.failures >= 0 &&
    optionalTimestamp(value.until) &&
    optionalTimestamp(value.probe_until) &&
    optionalTimestamp(value.updated_at)
  );
}

/** A response that fails this check is unsupported, not an empty answer. */
export function isRoutingCircuits(value: unknown): value is RoutingCircuits {
  return record(value) && Array.isArray(value.circuits) && value.circuits.every(validCircuit);
}

/** A fingerprint is unreadable in full; its first characters identify it. */
export function shortCircuitId(value: string): string {
  return value.length > 8 ? value.slice(0, 8) : value;
}

const STATE_ORDER: Record<RoutingCircuitState, number> = {
  open: 0,
  half_open: 1,
  closed: 2,
};

/** Blocking circuits first, then by key so equal states do not reorder on refresh. */
export function sortCircuits(circuits: readonly RoutingCircuit[]): RoutingCircuit[] {
  return [...circuits].sort(
    (left, right) =>
      STATE_ORDER[left.state] - STATE_ORDER[right.state] ||
      left.resource_key.localeCompare(right.resource_key),
  );
}

/** Whether the circuit currently refuses dispatch. */
export function isCircuitBlocking(circuit: RoutingCircuit, now = Date.now()): boolean {
  return (
    circuit.state === "half_open" ||
    (circuit.state === "open" && (!circuit.until || Date.parse(circuit.until) > now))
  );
}

/** How many circuits are blocking right now; closed rows are history. */
export function blockingCircuitCount(
  circuits: readonly RoutingCircuit[],
  now = Date.now(),
): number {
  return circuits.filter((circuit) => isCircuitBlocking(circuit, now)).length;
}
