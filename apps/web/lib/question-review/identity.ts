import { effectiveCompareState, type CompareState } from "./compare";

/**
 * What the stage shows, by comparison slot. Wipe draws A left of the divider
 * and B right of it; Onion skin draws B over A.
 */
export type VisibleIdentity =
  | { kind: "one"; slot: "a" | "b" }
  | { kind: "sides"; wipe: number }
  | { kind: "panes" }
  | { kind: "over"; opacity: number }
  | { kind: "versus" };

/** Derived from the displayed state, so it cannot lag a flip, swap, step, or hold. */
export function visibleIdentity(state: CompareState, holding: boolean): VisibleIdentity {
  const shown = effectiveCompareState(state, holding);
  switch (shown.mode) {
    case "toggle":
      return { kind: "one", slot: shown.showB ? "b" : "a" };
    case "wipe":
      return { kind: "sides", wipe: shown.wipe };
    case "side":
      return { kind: "panes" };
    case "onion":
      return { kind: "over", opacity: Math.round(shown.opacity * 100) };
    case "diff":
      return { kind: "versus" };
  }
}

/** Narrowest side of the Wipe divider, in CSS pixels, that still fits a name on the handle. */
export const WIPE_LABEL_MIN_SIDE_PX = 32;

/**
 * Which Wipe names fit beside the divider: A's on the left side, B's on the
 * right. An unmeasured stage shows both.
 */
export function wipeLabelSides(wipe: number, stageWidth: number): { a: boolean; b: boolean } {
  if (stageWidth <= 0) return { a: true, b: true };
  return {
    a: wipe * stageWidth >= WIPE_LABEL_MIN_SIDE_PX,
    b: (1 - wipe) * stageWidth >= WIPE_LABEL_MIN_SIDE_PX,
  };
}

const WIPE_NAME_MAX_LETTERS = 12;
const WIPE_NAME_MIN_LETTERS = 3;
/** Stage height kept clear of the handle's names: its bottom offset, padding, and top margin. */
const WIPE_NAME_RESERVED_PX = 112;
const WIPE_LETTER_PX = 12;

/** How many stacked letters a Wipe name gets on a stage of this height. */
export function wipeNameMaxLetters(stageHeight: number): number {
  if (stageHeight <= 0) return WIPE_NAME_MAX_LETTERS;
  const fit = Math.floor((stageHeight - WIPE_NAME_RESERVED_PX) / WIPE_LETTER_PX);
  return Math.min(WIPE_NAME_MAX_LETTERS, Math.max(WIPE_NAME_MIN_LETTERS, fit));
}

/** A Wipe name as upright letters, read top to bottom, cut to `max` with a vertical ellipsis. */
export function wipeNameLetters(name: string, max = WIPE_NAME_MAX_LETTERS): string[] {
  const letters = Array.from(name.trim());
  if (letters.length <= max) return letters;
  return [...letters.slice(0, Math.max(1, max - 1)), "\u22ee"];
}
