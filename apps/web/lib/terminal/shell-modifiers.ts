import { create } from "zustand";

export type ModifierState = { latched: boolean; sticky: boolean };

export type ShellModifiersStore = {
  ctrl: ModifierState;
  shift: ModifierState;
  toggleCtrl: () => void;
  toggleShift: () => void;
  consumeCtrl: () => void;
  consumeShift: () => void;
  reset: () => void;
};

const INITIAL: ModifierState = { latched: false, sticky: false };

function nextToggle(cur: ModifierState): ModifierState {
  if (cur.sticky) return { latched: false, sticky: false };
  if (cur.latched) return { latched: true, sticky: true };
  return { latched: true, sticky: false };
}

function nextConsume(cur: ModifierState): ModifierState {
  if (cur.sticky) return cur;
  return { latched: false, sticky: false };
}

/**
 * Transient, module-level modifier state for the mobile terminal key-bar.
 * Only one terminal is typed into at a time, so per-session scoping isn't
 * needed. Shared between `MobileTerminalKeybar` (UI + toggles) and the xterm
 * `onData` path (consumes on every keystroke).
 */
export const useShellModifiersStore = create<ShellModifiersStore>((set) => ({
  ctrl: INITIAL,
  shift: INITIAL,
  toggleCtrl: () => set((s) => ({ ctrl: nextToggle(s.ctrl) })),
  toggleShift: () => set((s) => ({ shift: nextToggle(s.shift) })),
  consumeCtrl: () => set((s) => ({ ctrl: nextConsume(s.ctrl) })),
  consumeShift: () => set((s) => ({ shift: nextConsume(s.shift) })),
  reset: () => set({ ctrl: INITIAL, shift: INITIAL }),
}));

export function isActive(m: ModifierState): boolean {
  return m.latched || m.sticky;
}
