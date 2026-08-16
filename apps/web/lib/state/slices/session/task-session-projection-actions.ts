import type { StateCreator } from "zustand";
import type { TaskSession } from "@/lib/types/http";
import type { SessionSlice } from "./types";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

type PendingActionProjection = Pick<
  TaskSession,
  "pending_action" | "pending_action_revision" | "pending_action_retired_epochs"
>;

const MAX_RETIRED_EPOCHS = 8;

function rememberRetiredEpoch(epochs: string[], epoch: string): string[] {
  return [...epochs.filter((candidate) => candidate !== epoch), epoch].slice(-MAX_RETIRED_EPOCHS);
}

function revisionMayReplaceExisting(
  incoming: TaskSession["pending_action_revision"],
  existing: TaskSession["pending_action_revision"],
  retiredEpochs: string[],
): incoming is NonNullable<TaskSession["pending_action_revision"]> {
  if (!incoming || retiredEpochs.includes(incoming.epoch)) return false;
  if (!existing || incoming.epoch !== existing.epoch) return true;
  return incoming.sequence >= existing.sequence;
}

function retiredEpochsAfterReplacement(
  incoming: NonNullable<TaskSession["pending_action_revision"]>,
  existing: TaskSession["pending_action_revision"],
  retiredEpochs: string[],
): string[] {
  if (!existing || incoming.epoch === existing.epoch) return retiredEpochs;
  return rememberRetiredEpoch(retiredEpochs, existing.epoch);
}

export function mergePendingActionProjection(
  existing: PendingActionProjection,
  incoming: PendingActionProjection,
): PendingActionProjection {
  const incomingRevision = incoming.pending_action_revision;
  const existingRevision = existing.pending_action_revision;
  const retiredEpochs = existing.pending_action_retired_epochs ?? [];
  if (revisionMayReplaceExisting(incomingRevision, existingRevision, retiredEpochs)) {
    const nextRetiredEpochs = retiredEpochsAfterReplacement(
      incomingRevision,
      existingRevision,
      retiredEpochs,
    );
    return {
      pending_action:
        incoming.pending_action === undefined ? existing.pending_action : incoming.pending_action,
      pending_action_revision: incomingRevision,
      pending_action_retired_epochs: nextRetiredEpochs.length > 0 ? nextRetiredEpochs : undefined,
    };
  }
  if (incomingRevision || existingRevision) {
    return {
      pending_action: existing.pending_action,
      pending_action_revision: existingRevision,
      pending_action_retired_epochs: retiredEpochs.length > 0 ? retiredEpochs : undefined,
    };
  }
  return {
    pending_action:
      incoming.pending_action === undefined ? existing.pending_action : incoming.pending_action,
    pending_action_revision: undefined,
    pending_action_retired_epochs: retiredEpochs.length > 0 ? retiredEpochs : undefined,
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
        session.pending_action_retired_epochs = projection.pending_action_retired_epochs;
        const sessionsByTask = draft.taskSessionsByTask.itemsByTaskId[session.task_id];
        if (sessionsByTask) {
          const match = sessionsByTask.find((item) => item.id === sessionId);
          if (match) {
            match.pending_action = projection.pending_action;
            match.pending_action_revision = projection.pending_action_revision;
            match.pending_action_retired_epochs = projection.pending_action_retired_epochs;
          }
        }
      }),
  };
}
