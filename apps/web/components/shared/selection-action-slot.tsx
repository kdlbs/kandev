"use client";

import { createContext, useContext, useMemo, useState, type ReactNode } from "react";
import { SELECTION_ACTION_DOCK_ATTRIBUTE } from "@/hooks/use-touch-selection-tracking";

type SelectionActionSlotContextValue = {
  slot: HTMLElement | null;
  setSlot: (element: HTMLElement | null) => void;
};

const SelectionActionSlotContext = createContext<SelectionActionSlotContextValue | null>(null);

/**
 * Lets a surface with an existing toolbar host touch selection actions inline
 * instead of in a separate floating dock. Wrap the selectable content and the
 * toolbar that renders `SelectionActionSlot`.
 */
export function SelectionActionSlotProvider({ children }: { children: ReactNode }) {
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  const value = useMemo(() => ({ slot, setSlot }), [slot]);
  return (
    <SelectionActionSlotContext.Provider value={value}>
      {children}
    </SelectionActionSlotContext.Provider>
  );
}

/** Toolbar position for selection actions; renders nothing without a provider. */
export function SelectionActionSlot() {
  const context = useContext(SelectionActionSlotContext);
  if (!context) return null;
  return (
    <span
      ref={context.setSlot}
      data-testid="selection-action-slot"
      {...{ [SELECTION_ACTION_DOCK_ATTRIBUTE]: "" }}
      className="contents"
    />
  );
}

/** The mounted toolbar slot for touch selection actions, if any. */
export function useSelectionActionSlot(): HTMLElement | null {
  return useContext(SelectionActionSlotContext)?.slot ?? null;
}
