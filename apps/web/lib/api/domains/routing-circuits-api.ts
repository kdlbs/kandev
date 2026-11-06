import { fetchJson, type ApiRequestOptions } from "../client";

/** One dynamic-routing resource circuit, as the read-only settings route returns it. */
export type RoutingCircuitWire = {
  resource_key: string;
  scope: string;
  scope_value: string;
  state: string;
  code?: string;
  until?: string;
  probe_until?: string;
  failures: number;
  updated_at?: string;
};

export type RoutingCircuitsWire = { circuits: RoutingCircuitWire[] };

/**
 * Reads the circuits the router is acting on. The route never opens, closes,
 * retries, or reorders a circuit.
 */
export function getRoutingCircuits(options?: ApiRequestOptions) {
  return fetchJson<unknown>("/api/v1/routing/circuits", options);
}
