import { create } from "zustand";
import type { CopilotItemRef } from "@/lib/coordinator/copilot-id";

export type { CopilotItemRef, CopilotItemRefKind } from "@/lib/coordinator/copilot-id";

export type CopilotChip = { id: string; label: string; ref: CopilotItemRef };

export type CopilotEntry = {
  open: boolean;
  chip: CopilotChip | null;
  draft: string;
};

export const INITIAL_COPILOT_ENTRY: CopilotEntry = { open: false, chip: null, draft: "" };

type CopilotStoreState = {
  entries: Record<string, CopilotEntry>;
  getEntry: (coordinatorId: string) => CopilotEntry;
  setOpen: (coordinatorId: string, open: boolean) => void;
  /** `label` always equals `id` in phase 1. */
  askAboutThis: (coordinatorId: string, id: string, ref: CopilotItemRef, draft: string) => void;
  /** The draft is a one-shot seed: the consumer applies it once, then clears it. */
  clearDraft: (coordinatorId: string) => void;
  /** Removes the chip only; the composer's typed text is left as-is. */
  removeChip: (coordinatorId: string) => void;
  /** A 404 for the coordinator: clears the chip and draft at once, without
   *  changing `open`. */
  clearChipAndDraft: (coordinatorId: string) => void;
  /** Deletes the entry entirely, back to the initial value. */
  removeEntry: (coordinatorId: string) => void;
};

/**
 * Client-memory store for **Ask about this**, one entry per coordinator id:
 * `{open, chip, draft}` (`docs/specs/coordinator/system-design/
 * copilot-panel.md#ask-about-this`). Deliberately not persisted (no
 * `zustand/middleware persist`): a reload starts every entry from the
 * initial value, and navigating between Needs you and Queue for one
 * coordinator keeps its entry because both routes read this same
 * module-level store.
 */
export const useCopilotStore = create<CopilotStoreState>()((set, get) => ({
  entries: {},
  getEntry: (coordinatorId) => get().entries[coordinatorId] ?? INITIAL_COPILOT_ENTRY,
  setOpen: (coordinatorId, open) =>
    set((state) => ({
      entries: {
        ...state.entries,
        [coordinatorId]: { ...(state.entries[coordinatorId] ?? INITIAL_COPILOT_ENTRY), open },
      },
    })),
  askAboutThis: (coordinatorId, id, ref, draft) =>
    set((state) => ({
      entries: {
        ...state.entries,
        [coordinatorId]: { open: true, chip: { id, label: id, ref }, draft },
      },
    })),
  clearDraft: (coordinatorId) =>
    set((state) => {
      const entry = state.entries[coordinatorId];
      if (!entry) return state;
      return { entries: { ...state.entries, [coordinatorId]: { ...entry, draft: "" } } };
    }),
  removeChip: (coordinatorId) =>
    set((state) => {
      const entry = state.entries[coordinatorId];
      if (!entry) return state;
      return { entries: { ...state.entries, [coordinatorId]: { ...entry, chip: null } } };
    }),
  clearChipAndDraft: (coordinatorId) =>
    set((state) => {
      const entry = state.entries[coordinatorId] ?? INITIAL_COPILOT_ENTRY;
      return {
        entries: { ...state.entries, [coordinatorId]: { ...entry, chip: null, draft: "" } },
      };
    }),
  removeEntry: (coordinatorId) =>
    set((state) => {
      if (!(coordinatorId in state.entries)) return state;
      const entries = { ...state.entries };
      delete entries[coordinatorId];
      return { entries };
    }),
}));

/** Reactive per-coordinator entry read, for components; `getEntry` above
 *  reads a snapshot outside React. */
export function useCopilotEntry(coordinatorId: string): CopilotEntry {
  return useCopilotStore((state) => state.entries[coordinatorId] ?? INITIAL_COPILOT_ENTRY);
}
