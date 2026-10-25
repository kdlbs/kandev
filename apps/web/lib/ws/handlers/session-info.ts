import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { SessionInfoPayload } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import {
  getAgentGoal,
  mergeAgentGoalMetadata,
  parseAgentGoal,
  type AgentGoal,
  type AgentGoalReconciliation,
} from "@/lib/agent-goal";
import type { TaskSession } from "@/lib/types/http";

export function registerSessionInfoHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "session.info_updated": (message) => {
      const payload = message.payload as SessionInfoPayload | undefined;
      if (!payload?.session_id) return;
      const existing = store.getState().taskSessions.items[payload.session_id];
      if (!existing) return;
      const existingACP = readExistingACP(existing.metadata?.acp);
      if (isStaleSessionInfoUpdate(payload.session_updated_at, existingACP.updated_at)) return;
      store.getState().setTaskSession(buildNextSessionInfoSession(existing, payload));
    },
  };
}

function buildNextSessionInfoSession(
  existing: TaskSession,
  payload: SessionInfoPayload,
): TaskSession {
  const existingACP = readExistingACP(existing.metadata?.acp);
  const attachmentChanged =
    Boolean(payload.acp_session_id) &&
    Boolean(existingACP.session_id) &&
    payload.acp_session_id !== existingACP.session_id;
  const meta =
    payload.session_meta !== undefined || attachmentChanged
      ? mergeAgentGoalMetadata(existingACP.meta, payload.session_meta, {
          attachmentChanged,
          source: "live",
          reconciliation: existing.goal_reconciliation,
        })
      : existingACP.meta;
  const nextSession: TaskSession = {
    ...existing,
    metadata: {
      ...(existing.metadata ?? {}),
      acp: {
        session_id: payload.acp_session_id || existingACP.session_id,
        title: payload.session_title || existingACP.title,
        updated_at: payload.session_updated_at || existingACP.updated_at,
        meta,
      },
    },
  };
  const reconciliation = nextLiveGoalReconciliation(
    existing.goal_reconciliation,
    getAgentGoal(existingACP.meta),
    payload.session_meta,
    attachmentChanged,
    meta,
  );
  if (reconciliation) {
    nextSession.goal_reconciliation =
      attachmentChanged || hasGoalField(payload.session_meta)
        ? withGoalSnapshotTimestamp(reconciliation, payload.session_updated_at)
        : reconciliation;
  }
  return nextSession;
}

function withGoalSnapshotTimestamp(
  reconciliation: AgentGoalReconciliation,
  updatedAt: string | undefined,
): AgentGoalReconciliation {
  return updatedAt ? { ...reconciliation, sourceUpdatedAt: updatedAt } : reconciliation;
}

function nextLiveGoalReconciliation(
  current: AgentGoalReconciliation | undefined,
  currentGoal: AgentGoal | null,
  incomingMeta: Record<string, unknown> | undefined,
  attachmentChanged: boolean,
  mergedMeta: Record<string, unknown>,
): AgentGoalReconciliation | undefined {
  const revision = (current?.revision ?? 0) + 1;
  const currentWatermark =
    current?.watermark ??
    (currentGoal ? { createdAt: currentGoal.createdAt, updatedAt: currentGoal.updatedAt } : null);
  if (attachmentChanged) return reconciliationForAttachment(revision, mergedMeta);
  if (!hasGoalField(incomingMeta)) {
    return reconciliationForAbsentGoal(current, currentGoal, currentWatermark);
  }
  if (incomingMeta?.goal === null) {
    return { revision, cleared: true, watermark: currentWatermark };
  }
  return reconciliationForLiveGoal(current, currentGoal, incomingMeta ?? {}, mergedMeta, revision);
}

function hasGoalField(meta: Record<string, unknown> | undefined): boolean {
  return Object.prototype.hasOwnProperty.call(meta ?? {}, "goal");
}

function reconciliationForAttachment(
  revision: number,
  mergedMeta: Record<string, unknown>,
): AgentGoalReconciliation {
  const accepted = parseAgentGoal(mergedMeta.goal);
  return {
    revision,
    cleared: !accepted,
    watermark: accepted ? { createdAt: accepted.createdAt, updatedAt: accepted.updatedAt } : null,
  };
}

function reconciliationForAbsentGoal(
  current: AgentGoalReconciliation | undefined,
  currentGoal: AgentGoal | null,
  currentWatermark: AgentGoalReconciliation["watermark"],
): AgentGoalReconciliation | undefined {
  if (current) return current;
  if (!currentGoal) return undefined;
  return { revision: 1, cleared: false, watermark: currentWatermark };
}

function reconciliationForLiveGoal(
  current: AgentGoalReconciliation | undefined,
  currentGoal: AgentGoal | null,
  incomingMeta: Record<string, unknown>,
  mergedMeta: Record<string, unknown>,
  revision: number,
): AgentGoalReconciliation | undefined {
  const accepted = parseAgentGoal(mergedMeta.goal);
  const incoming = parseAgentGoal(incomingMeta.goal);
  if (!accepted || !incoming) return current ?? goalReconciliationFrom(currentGoal);
  if (accepted.createdAt !== incoming.createdAt || accepted.updatedAt !== incoming.updatedAt) {
    return current ?? goalReconciliationFrom(currentGoal);
  }
  return {
    revision,
    cleared: false,
    watermark: { createdAt: accepted.createdAt, updatedAt: accepted.updatedAt },
  };
}

function goalReconciliationFrom(goal: AgentGoal | null): AgentGoalReconciliation | undefined {
  if (!goal) return undefined;
  return {
    revision: 1,
    cleared: false,
    watermark: { createdAt: goal.createdAt, updatedAt: goal.updatedAt },
  };
}

function isStaleSessionInfoUpdate(
  incomingUpdatedAt: string | undefined,
  existingUpdatedAt: string,
) {
  if (!incomingUpdatedAt || !existingUpdatedAt) return false;
  const incoming = Date.parse(incomingUpdatedAt);
  const existing = Date.parse(existingUpdatedAt);
  if (Number.isNaN(incoming) || Number.isNaN(existing)) return false;
  return incoming < existing;
}

function readExistingACP(value: unknown): {
  session_id: string;
  title: string;
  updated_at: string;
  meta: Record<string, unknown>;
} {
  if (!value || typeof value !== "object") {
    return { session_id: "", title: "", updated_at: "", meta: {} };
  }
  const record = value as Record<string, unknown>;
  return {
    session_id: typeof record.session_id === "string" ? record.session_id : "",
    title: typeof record.title === "string" ? record.title : "",
    updated_at: typeof record.updated_at === "string" ? record.updated_at : "",
    meta:
      record.meta && typeof record.meta === "object"
        ? (record.meta as Record<string, unknown>)
        : {},
  };
}
