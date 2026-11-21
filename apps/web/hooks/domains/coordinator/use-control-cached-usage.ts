import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import { NativeControlContractError } from "@/lib/api/domains/control-native-api";
import { readCachedProfileExecutionUsage } from "@/lib/api/domains/provider-usage-api";
import { isCachedProfileUsage } from "@/lib/control-ui/usage";
import { controlReadFailure, retainControlRows, useControlResource } from "./use-control-resource";

/** The caller supplies only workspaces with the new recorded-control API contract. */
export function useControlCachedUsage(workspaceIds: string[], revision: object | undefined) {
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const ids = JSON.stringify(
    [
      ...new Set(
        profiles
          .filter(
            (profile) =>
              workspaceIds.length > 0 &&
              (!profile.workspace_id || workspaceIds.includes(profile.workspace_id)),
          )
          .map((profile) => profile.id),
      ),
    ].sort(),
  );
  const read = useCallback(
    async (signal: AbortSignal) =>
      Promise.all(
        (JSON.parse(ids) as string[]).map(async (id) => {
          try {
            const response = await readCachedProfileExecutionUsage(id, {
              cache: "no-store",
              init: { signal },
            });
            if (!isCachedProfileUsage(response, id)) {
              // i18n-exempt: diagnostic only; the UI displays the localized read error.
              throw new NativeControlContractError("Incompatible cached profile usage response");
            }
            return {
              id,
              status: "ready" as const,
              entries: response.executions.filter(
                (entry) => entry.report.model?.toLowerCase() !== "dynamic",
              ),
            };
          } catch (error) {
            return { id, status: controlReadFailure(error), entries: [] };
          }
        }),
      ),
    [ids],
  );
  return useControlResource(read, revision, retainControlRows);
}
