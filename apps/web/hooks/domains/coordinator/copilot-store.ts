import { useMemo } from "react";
import { create } from "zustand";

export type CopilotChip = { id: string; label: string };

export type CopilotEntry = {
  open: boolean;
  chip: CopilotChip | null;
  draft: string;
};

export const INITIAL_COPILOT_ENTRY: CopilotEntry = { open: false, chip: null, draft: "" };

type CopilotSlot = CopilotEntry & {
  /** The coordinator the slot belongs to; null while nothing has been opened. */
  coordinatorId: string | null;
  /** True once the composer's stored draft text was cleared for this slot. */
  draftsSwept: boolean;
};

const INITIAL_SLOT: CopilotSlot = {
  ...INITIAL_COPILOT_ENTRY,
  coordinatorId: null,
  draftsSwept: false,
};

type CopilotStoreState = CopilotSlot & {
  getEntry: (coordinatorId: string) => CopilotEntry;
  setOpen: (coordinatorId: string, open: boolean) => void;
  /** `label` always equals `id` in phase 1. */
  askAboutThis: (coordinatorId: string, id: string, draft: string) => void;
  /** The draft is a one-shot seed: the consumer applies it once, then clears it. */
  clearDraft: (coordinatorId: string) => void;
  /** Removes the chip only; the composer's typed text is left as-is. */
  removeChip: (coordinatorId: string) => void;
  /** A 404 for the coordinator: clears the chip and draft at once, without
   *  changing `open`. */
  clearChipAndDraft: (coordinatorId: string) => void;
  /** Back to the initial value when the slot belongs to `coordinatorId`. */
  removeEntry: (coordinatorId: string) => void;
  /** Keeps the slot only while `coordinatorId` is the coordinator it belongs
   *  to; any other coordinator, or null for a non-coordinator path, resets it. */
  keepOnlyFor: (coordinatorId: string | null) => void;
  /** Records that the stored draft text was cleared, when the slot belongs to
   *  `coordinatorId`; never takes the slot over. */
  markDraftsSwept: (coordinatorId: string) => void;
};

/** The slot's entry as seen by `coordinatorId`: a slot that belongs to another
 *  coordinator reads as the initial value. */
function slotFor(state: CopilotSlot, coordinatorId: string): CopilotSlot {
  return state.coordinatorId === coordinatorId ? state : { ...INITIAL_SLOT, coordinatorId };
}

function entryOf(slot: CopilotSlot): CopilotEntry {
  return { open: slot.open, chip: slot.chip, draft: slot.draft };
}

/**
 * Client-memory store for the copilot panel and **Ask about this**: a single
 * `{coordinatorId, open, chip, draft}` slot (`docs/specs/coordinator/
 * system-design/copilot-popover.md#ask-about-this`). Deliberately not
 * persisted: a reload starts from the initial value. The slot survives moving
 * between Needs you and Queue for one coordinator and closing and reopening
 * the panel; acting for a different coordinator replaces it, and
 * `keepOnlyFor` resets it on any other path.
 */
export const useCopilotStore = create<CopilotStoreState>()((set, get) => ({
  ...INITIAL_SLOT,
  getEntry: (coordinatorId) => {
    const state = get();
    return state.coordinatorId === coordinatorId ? entryOf(state) : INITIAL_COPILOT_ENTRY;
  },
  setOpen: (coordinatorId, open) => set((state) => ({ ...slotFor(state, coordinatorId), open })),
  askAboutThis: (coordinatorId, id, draft) =>
    set((state) => ({
      ...slotFor(state, coordinatorId),
      open: true,
      chip: { id, label: id },
      draft,
    })),
  clearDraft: (coordinatorId) =>
    set((state) => (state.coordinatorId === coordinatorId ? { draft: "" } : state)),
  removeChip: (coordinatorId) =>
    set((state) => (state.coordinatorId === coordinatorId ? { chip: null } : state)),
  clearChipAndDraft: (coordinatorId) =>
    set((state) => ({ ...slotFor(state, coordinatorId), chip: null, draft: "" })),
  removeEntry: (coordinatorId) =>
    set((state) => (state.coordinatorId === coordinatorId ? INITIAL_SLOT : state)),
  keepOnlyFor: (coordinatorId) =>
    set((state) =>
      state.coordinatorId === null || state.coordinatorId === coordinatorId ? state : INITIAL_SLOT,
    ),
  markDraftsSwept: (coordinatorId) =>
    set((state) => (state.coordinatorId === coordinatorId ? { draftsSwept: true } : state)),
}));

/** Reactive entry read for `coordinatorId`, for components; `getEntry` above
 *  reads a snapshot outside React. */
export function useCopilotEntry(coordinatorId: string): CopilotEntry {
  const owned = useCopilotStore((state) => state.coordinatorId === coordinatorId);
  const open = useCopilotStore((state) => owned && state.open);
  const chip = useCopilotStore((state) => (owned ? state.chip : null));
  const draft = useCopilotStore((state) => (owned ? state.draft : ""));
  return useMemo(() => ({ open, chip, draft }), [open, chip, draft]);
}

export function useCopilotDraftsSwept(coordinatorId: string): boolean {
  return useCopilotStore((state) => state.coordinatorId === coordinatorId && state.draftsSwept);
}
