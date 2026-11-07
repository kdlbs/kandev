import { useCallback, useMemo } from "react";
import type { ApiRequestOptions } from "@/lib/api/client";
import {
  listClarificationInbox,
  listEarlierTurnClarificationInbox,
  listInboxRecoveryBlocks,
} from "@/lib/api/domains/clarification-inbox-api";
import { listFailedInbox } from "@/lib/api/domains/failed-inbox-api";
import type { ClarificationInboxBundle, InboxRecoveryBlock } from "@/lib/types/clarification-inbox";
import type { FailedInboxRow } from "@/lib/types/failed-inbox";
import type {
  ControlInboxSection,
  ControlInboxWorkspaceRead,
} from "@/components/integrated-control/control-inbox-model";
import { controlReadFailure, useControlResource } from "./use-control-resource";

async function section<T>(
  read: () => Promise<{ items: T[]; truncated: boolean }>,
): Promise<ControlInboxSection<T>> {
  try {
    return { status: "ready", ...(await read()) };
  } catch (error) {
    const status = controlReadFailure(error);
    return {
      status: status === "loading" || status === "ready" ? "error" : status,
      items: [],
      truncated: false,
    };
  }
}

/** Reads every Needs-you section of one workspace; one failing section never hides the others. */
export async function readWorkspaceInbox(
  workspaceId: string,
  options: ApiRequestOptions,
): Promise<ControlInboxWorkspaceRead> {
  const [answerable, earlier, recovery, failed] = await Promise.all([
    section<ClarificationInboxBundle>(async () => {
      const page = await listClarificationInbox(workspaceId, options);
      return { items: page.bundles, truncated: Boolean(page.next_cursor) };
    }),
    section<ClarificationInboxBundle>(async () => {
      const page = await listEarlierTurnClarificationInbox(workspaceId, options);
      return { items: page.bundles, truncated: Boolean(page.next_cursor) };
    }),
    section<InboxRecoveryBlock>(async () => {
      const page = await listInboxRecoveryBlocks(workspaceId, options);
      return { items: page.blocks, truncated: page.truncated };
    }),
    section<FailedInboxRow>(async () => {
      const page = await listFailedInbox(workspaceId, options);
      return { items: page.rows, truncated: page.truncated };
    }),
  ]);
  return { workspaceId, answerable, earlier, recovery, failed };
}

/**
 * The Needs-you sections of every listed workspace, re-read whenever the
 * revision changes (a control refresh or an answered row). An undefined
 * revision reads nothing.
 */
export function useControlInboxReads(workspaceIds: string[], revision: object | undefined) {
  const key = JSON.stringify([...new Set(workspaceIds)].sort());
  const read = useCallback(
    (signal: AbortSignal) =>
      Promise.all(
        (JSON.parse(key) as string[]).map((id) =>
          readWorkspaceInbox(id, { cache: "no-store", init: { signal } }),
        ),
      ),
    [key],
  );
  const result = useControlResource(read, revision);
  const order = useMemo(
    () => new Map(workspaceIds.map((id, index) => [id, index])),
    [workspaceIds],
  );
  const reads = useMemo(
    () =>
      [...(result.data ?? [])].sort(
        (a, b) => (order.get(a.workspaceId) ?? 0) - (order.get(b.workspaceId) ?? 0),
      ),
    [result.data, order],
  );
  return { ...result, reads };
}
