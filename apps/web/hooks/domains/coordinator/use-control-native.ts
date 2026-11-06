import { useCallback } from "react";
import type { ApiRequestOptions } from "@/lib/api/client";
import { readNativeDashboard } from "@/lib/api/domains/control-native-api";
import { useControlResource, controlReadFailure, retainControlRows } from "./use-control-resource";

function useWorkspaceReads<T>(
  workspaceIds: string[],
  revision: object | undefined,
  reader: (id: string, options: ApiRequestOptions) => Promise<T>,
) {
  const ids = JSON.stringify([...new Set(workspaceIds)].sort());
  const read = useCallback(
    (signal: AbortSignal) =>
      Promise.all(
        (JSON.parse(ids) as string[]).map(async (id) => {
          try {
            return {
              id,
              status: "ready" as const,
              data: await reader(id, { cache: "no-store", init: { signal } }),
            };
          } catch (error) {
            return { id, status: controlReadFailure(error) };
          }
        }),
      ),
    [ids, reader],
  );
  return useControlResource(read, revision, retainControlRows);
}
export const useNativeDashboards = (ids: string[], revision: object | undefined) =>
  useWorkspaceReads(ids, revision, readNativeDashboard);
