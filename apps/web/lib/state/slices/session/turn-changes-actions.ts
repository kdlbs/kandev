import type { StateCreator } from "zustand";
import type { Draft } from "immer";
import type { SessionSlice } from "./types";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";

type ImmerSet = Parameters<
  StateCreator<SessionSlice, [["zustand/immer", never]], [], SessionSlice>
>[0];

function upsertByRevision(
  summaries: Draft<TurnChangeSetSummary>[],
  incoming: TurnChangeSetSummary,
): void {
  const index = summaries.findIndex((summary) => summary.id === incoming.id);
  if (index >= 0) {
    if (summaries[index].revision >= incoming.revision) return;
    summaries[index] = incoming as Draft<TurnChangeSetSummary>;
  } else {
    summaries.push(incoming as Draft<TurnChangeSetSummary>);
  }
  summaries.sort((a, b) => a.turn_ordinal - b.turn_ordinal || a.id.localeCompare(b.id));
}

export function buildTurnChangeActions(set: ImmerSet) {
  return {
    mergeTurnChangePage: (
      sessionId: string,
      summaries: TurnChangeSetSummary[],
      page: { offset: number; nextOffset?: number },
    ) =>
      set((draft) => {
        const current = (draft.turnChanges.bySession[sessionId] ??= []);
        for (const summary of summaries) upsertByRevision(current, summary);
        if (page.offset === 0) draft.turnChanges.loadedBySession[sessionId] = true;
        draft.turnChanges.nextOffsetBySession[sessionId] = page.nextOffset ?? null;
      }),
    mergeTurnChangeSummary: (sessionId: string, summary: TurnChangeSetSummary) =>
      set((draft) => {
        const current = (draft.turnChanges.bySession[sessionId] ??= []);
        upsertByRevision(current, summary);
      }),
    markTurnChangesLoaded: (sessionId: string, nextOffset: number | undefined) =>
      set((draft) => {
        draft.turnChanges.loadedBySession[sessionId] = true;
        draft.turnChanges.nextOffsetBySession[sessionId] = nextOffset ?? null;
      }),
  };
}
