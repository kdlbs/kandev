import type { StateCreator } from "zustand";
import type { TaskSession } from "@/lib/types/http";
import type { SessionSlice } from "./types";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

type PendingActionProjection = Pick<TaskSession, "pending_action" | "pending_action_revision">;

function parseEpoch(epoch: string): bigint | null {
  if (!/^\d+$/.test(epoch)) return null;
  return BigInt(epoch);
}

function revisionMayReplaceExisting(
  incoming: TaskSession["pending_action_revision"],
  existing: TaskSession["pending_action_revision"],
): incoming is NonNullable<TaskSession["pending_action_revision"]> {
  if (!incoming) return false;
  const incomingEpoch = parseEpoch(incoming.epoch);
  if (incomingEpoch === null) return false;
  if (!existing) return true;
  const existingEpoch = parseEpoch(existing.epoch);
  if (existingEpoch === null || incomingEpoch > existingEpoch) return true;
  if (incomingEpoch < existingEpoch) return false;
  return incoming.sequence >= existing.sequence;
}

export function mergePendingActionProjection(
  existing: PendingActionProjection,
  incoming: PendingActionProjection,
): PendingActionProjection {
  const incomingRevision = incoming.pending_action_revision;
  const existingRevision = existing.pending_action_revision;
  if (revisionMayReplaceExisting(incomingRevision, existingRevision)) {
    return {
      pending_action:
        incoming.pending_action === undefined ? existing.pending_action : incoming.pending_action,
      pending_action_revision: incomingRevision,
    };
  }
  if (incomingRevision || existingRevision) {
    return {
      pending_action: existing.pending_action,
      pending_action_revision: existingRevision,
    };
  }
  return {
    pending_action:
      incoming.pending_action === undefined ? existing.pending_action : incoming.pending_action,
    pending_action_revision: undefined,
  };
}

export function buildTaskSessionProjectionActions(set: ImmerSet) {
  return {
    setTaskSessionPendingAction: (
      sessionId: string,
      pendingAction: Parameters<SessionSlice["setTaskSessionPendingAction"]>[1],
      revision: Parameters<SessionSlice["setTaskSessionPendingAction"]>[2],
    ) =>
      set((draft) => {
        const session = draft.taskSessions.items[sessionId];
        if (!session) return;
        const projection = mergePendingActionProjection(session, {
          pending_action: pendingAction,
          pending_action_revision: revision,
        });
        session.pending_action = projection.pending_action;
        session.pending_action_revision = projection.pending_action_revision;
        const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[session.task_id];
        if (sessionsByTask) {
          const match = sessionsByTask.find((item) => item.id === sessionId);
          if (match) {
            match.pending_action = projection.pending_action;
            match.pending_action_revision = projection.pending_action_revision;
          }
        }
      }),
  };
}
