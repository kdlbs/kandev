/**
 * Keyboard shortcut constants and key definitions
 */

// Modifier keys
export const MODIFIER_KEYS = {
  CTRL: "Control",
  CMD: "Meta",
  ALT: "Alt",
  SHIFT: "Shift",
} as const;

// Common keys
export const KEYS = {
  ENTER: "Enter",
  ESCAPE: "Escape",
  SPACE: " ",
  TAB: "Tab",
  BACKSPACE: "Backspace",
  DELETE: "Delete",
  ARROW_UP: "ArrowUp",
  ARROW_DOWN: "ArrowDown",
  ARROW_LEFT: "ArrowLeft",
  ARROW_RIGHT: "ArrowRight",
  // Letters
  A: "a",
  B: "b",
  C: "c",
  D: "d",
  E: "e",
  F: "f",
  G: "g",
  H: "h",
  I: "i",
  J: "j",
  K: "k",
  L: "l",
  M: "m",
  N: "n",
  O: "o",
  P: "p",
  Q: "q",
  R: "r",
  S: "s",
  T: "t",
  U: "u",
  V: "v",
  W: "w",
  X: "x",
  Y: "y",
  Z: "z",
  // Numbers
  ZERO: "0",
  ONE: "1",
  TWO: "2",
  THREE: "3",
  FOUR: "4",
  FIVE: "5",
  SIX: "6",
  SEVEN: "7",
  EIGHT: "8",
  NINE: "9",
  // Symbols
  SLASH: "/",
} as const;

// Platform types
export type Platform = "mac" | "windows" | "linux" | "unknown";

// Modifier key type
export type ModifierKey = keyof typeof MODIFIER_KEYS;

// Key type
export type Key = (typeof KEYS)[keyof typeof KEYS];

/**
 * Keyboard shortcut definition
 */
export type KeyboardShortcut = {
  key: Key;
  modifiers?: {
    ctrl?: boolean;
    cmd?: boolean;
    alt?: boolean;
    shift?: boolean;
    /** Use Cmd on Mac, Ctrl on Windows/Linux */
    ctrlOrCmd?: boolean;
  };
};

/**
 * Common keyboard shortcuts used across the app
 */
export const SHORTCUTS = {
  SUBMIT: {
    key: KEYS.ENTER,
    modifiers: { ctrlOrCmd: true },
  },
  SUBMIT_ENTER: {
    key: KEYS.ENTER,
  },
  SAVE: {
    key: KEYS.S,
    modifiers: { ctrlOrCmd: true },
  },
  CANCEL: {
    key: KEYS.ESCAPE,
  },
  SEARCH: {
    key: KEYS.K,
    modifiers: { ctrlOrCmd: true },
  },
  NEW_TASK: {
    key: KEYS.N,
    modifiers: { ctrlOrCmd: true },
  },
  TOGGLE_SIDEBAR: {
    key: KEYS.B,
    modifiers: { ctrlOrCmd: true },
  },
  FOCUS_INPUT: {
    key: KEYS.SLASH,
  },
  COMMAND_PANEL: {
    key: KEYS.P,
    modifiers: { ctrlOrCmd: true },
  },
  COMMAND_PANEL_SHIFT: {
    key: KEYS.P,
    modifiers: { ctrlOrCmd: true, shift: true },
  },
  TOGGLE_PLAN_MODE: {
    key: KEYS.TAB,
    modifiers: { shift: true },
  },
} as const;
