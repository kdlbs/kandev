import { useCallback } from "react";
import type { ApiRequestOptions } from "@/lib/api/client";
import { listCoordinatorStalls } from "@/lib/api/domains/coordinator-api";
import {
  controlReadFailure,
  retainControlRows,
  useControlResource,
  type ControlReadStatus,
} from "./use-control-resource";

/** One workspace's recorded coordinator stalls; the count, never the reasons. */
export type ControlStallRow = {
  id: string;
  status: ControlReadStatus;
  count?: number;
};

async function readStallCount(
  workspaceId: string,
  options: ApiRequestOptions,
): Promise<ControlStallRow> {
  try {
    const { stalls } = await listCoordinatorStalls(workspaceId, options);
    return { id: workspaceId, status: "ready", count: stalls.length };
  } catch (error) {
    return { id: workspaceId, status: controlReadFailure(error) };
  }
}

/** The existing coordinator-stalls route, read per workspace on the control snapshot. */
export function useControlStalls(workspaceIds: string[], revision: object | undefined) {
  const ids = JSON.stringify([...new Set(workspaceIds)].sort());
  const read = useCallback(
    (signal: AbortSignal) =>
      Promise.all(
        (JSON.parse(ids) as string[]).map((id) =>
          readStallCount(id, { cache: "no-store", init: { signal } }),
        ),
      ),
    [ids],
  );
  return useControlResource(read, revision, retainControlRows);
}
