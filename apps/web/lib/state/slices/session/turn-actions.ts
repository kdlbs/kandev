import type { StateCreator } from "zustand";
import type { SessionSlice } from "./types";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

export function buildTurnActions(set: ImmerSet) {
  return {
    addTurn: (turn: Parameters<SessionSlice["addTurn"]>[0]) =>
      set((draft) => {
        const sessionId = turn.session_id;
        if (!draft.turns.bySession[sessionId]) draft.turns.bySession[sessionId] = [];
        const existing = draft.turns.bySession[sessionId].find((item) => item.id === turn.id);
        if (!existing) {
          draft.turns.bySession[sessionId].push(turn);
          return;
        }
        Object.assign(existing, turn, { completed_at: existing.completed_at ?? turn.completed_at });
      }),
    completeTurn: (
      sessionId: Parameters<SessionSlice["completeTurn"]>[0],
      turnId: Parameters<SessionSlice["completeTurn"]>[1],
      completedAt: Parameters<SessionSlice["completeTurn"]>[2],
      metadata: Parameters<SessionSlice["completeTurn"]>[3],
    ) =>
      set((draft) => {
        const turn = draft.turns.bySession[sessionId]?.find((item) => item.id === turnId);
        if (turn) {
          turn.completed_at = completedAt;
          if (metadata) turn.metadata = metadata;
        }
        if (draft.turns.activeBySession[sessionId] === turnId) {
          draft.turns.activeBySession[sessionId] = null;
        }
      }),
    setActiveTurn: (
      sessionId: Parameters<SessionSlice["setActiveTurn"]>[0],
      turnId: Parameters<SessionSlice["setActiveTurn"]>[1],
    ) =>
      set((draft) => {
        if (!turnId) {
          draft.turns.activeBySession[sessionId] = null;
          return;
        }
        const turns = draft.turns.bySession[sessionId] ?? [];
        const next = turns.find((turn) => turn.id === turnId);
        if (!next || next.completed_at) return;

        const currentId = draft.turns.activeBySession[sessionId];
        const current = turns.find((turn) => turn.id === currentId);
        if (!current || current.completed_at) {
          draft.turns.activeBySession[sessionId] = turnId;
          return;
        }
        if (current.id === turnId) return;

        const currentStartedAt = Date.parse(current.started_at);
        const nextStartedAt = Date.parse(next.started_at);
        if (
          !Number.isNaN(currentStartedAt) &&
          !Number.isNaN(nextStartedAt) &&
          nextStartedAt > currentStartedAt
        ) {
          draft.turns.activeBySession[sessionId] = turnId;
        }
      }),
    reconcileWorkspaceSourcesAdopted: (
      sessionIds: Parameters<SessionSlice["reconcileWorkspaceSourcesAdopted"]>[0],
    ) =>
      set((draft) => {
        for (const sessionId of new Set(sessionIds)) {
          draft.turns.activeBySession[sessionId] = null;
        }
      }),
  };
}
