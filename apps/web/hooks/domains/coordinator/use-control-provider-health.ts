import { useCallback } from "react";
import { getProviderHealth } from "@/lib/api/domains/office-routing-api";
import { NativeControlContractError } from "@/lib/api/domains/control-native-api";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { controlReadFailure, retainControlRows, useControlResource } from "./use-control-resource";

/** Existing workspace-scoped stored health; never calls routing retry or preview. */
export function useControlProviderHealth(workspaceIds: string[], revision: object | undefined) {
  const ids = JSON.stringify([...new Set(workspaceIds)].sort());
  const read = useCallback(
    async (signal: AbortSignal) =>
      Promise.all(
        (JSON.parse(ids) as string[]).map(async (id) => {
          try {
            const response = await getProviderHealth(id, { cache: "no-store", init: { signal } });
            if (
              !Array.isArray(response.health) ||
              !response.health.every((row) => validHealth(row, id))
            ) {
              // i18n-exempt: diagnostic only; the UI displays the localized read error.
              throw new NativeControlContractError(
                "Incompatible workspace provider health response",
              );
            }
            return { id, status: "ready" as const, health: response.health };
          } catch (error) {
            return { id, status: controlReadFailure(error), health: [] as ProviderHealth[] };
          }
        }),
      ),
    [ids],
  );
  return useControlResource(read, revision, retainControlRows);
}

function validHealth(row: ProviderHealth, workspaceId: string) {
  return (
    row != null &&
    (!row.workspace_id || row.workspace_id === workspaceId) &&
    typeof row.provider_id === "string" &&
    row.provider_id.length > 0 &&
    ["provider", "model", "tier"].includes(row.scope) &&
    typeof row.scope_value === "string" &&
    ["healthy", "short_retry", "degraded", "user_action_required"].includes(row.state)
  );
}
