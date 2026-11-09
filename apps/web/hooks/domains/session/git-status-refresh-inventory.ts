import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionGitRefreshResponse } from "@/lib/ws/client";

type InventoryEvidence = {
  received: number;
  count: number;
  scopes: string[];
};

/**
 * Returns the complete scope set only when one correlated aggregate response
 * is a validated inventory: at least one usable snapshot, every eligible
 * snapshot accepted, and the response itself successful. Malformed,
 * truncated, mixed-source, or unrelated responses cannot retire a scope.
 */
export function authoritativeInventoryScopes(
  response: SessionGitRefreshResponse | null,
  outcome: InventoryEvidence | null,
): string[] | null {
  if (!response?.success || !outcome || outcome.received === 0) return null;
  if (outcome.count !== outcome.received) return null;
  return outcome.scopes;
}

export function reconcileInventoryFromSnapshot(
  store: StoreApi<AppState>,
  environmentId: string,
  inventoryGeneration: number,
  response: SessionGitRefreshResponse | null,
  outcome: InventoryEvidence | null,
): void {
  const scopes = authoritativeInventoryScopes(response, outcome);
  if (!scopes) return;
  store.getState().reconcileGitStatusInventory(environmentId, inventoryGeneration, scopes);
}
