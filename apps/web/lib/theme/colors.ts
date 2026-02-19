/**
 * Centralized theme color palette.
 *
 * Change colors here to affect Monaco editor, Pierre diffs, xterm terminal,
 * and any other component that references these values.
 *
 * CSS variables in globals.css (--background, --foreground, --card, etc.)
 * define the same palette for Tailwind / CSS consumers. Keep them in sync
 * when changing values here.
 */

// ---------------------------------------------------------------------------
// Core palette — dark
// ---------------------------------------------------------------------------
export const DARK = {
  bg: "#141414",
  fg: "#d4d4d4",
  card: "#1c1c1c",
  border: "#2a2a2a",
  hover: "#333333",
  popover: "#222222",
  lineHighlight: "#1c1c1c",
  lineNumber: "#555555",
  lineNumberActive: "#888888",
  selection: "#264f78",
  selectionInactive: "#3a3d41",
  cursor: "#d4d4d4",
  scrollbarShadow: "#00000000",
  scrollbarThumb: "#64646480",
  scrollbarThumbHover: "#82828299",
  scrollbarThumbActive: "#828282bb",
} as const;

// ---------------------------------------------------------------------------
// Core palette — light
// ---------------------------------------------------------------------------
export const LIGHT = {
  bg: "#ffffff",
  fg: "#1e1e1e",
  card: "#ffffff",
  lineHighlight: "#f5f5f5",
  lineNumber: "#c0c0c0",
  lineNumberActive: "#555555",
} as const;

// ---------------------------------------------------------------------------
// Git diff colors (shared across Monaco, Pierre, CSS --git-addition/deletion)
// ---------------------------------------------------------------------------
export const DIFF_COLORS = {
  addition: "#10b981", // emerald-500
  deletion: "#f43f5e", // rose-500
  /** Alpha variants for Monaco diff backgrounds */
  additionTextBg: "#10b98126",
  deletionTextBg: "#f43f5e26",
  additionLineBg: "#10b98115",
  deletionLineBg: "#f43f5e15",
} as const;

// ---------------------------------------------------------------------------
// Font stack
// ---------------------------------------------------------------------------
export const FONT = {
  mono: '"Geist Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
  size: 12,
  lineHeight: 18,
} as const;

// ---------------------------------------------------------------------------
// Pierre diffs — Shiki syntax-highlighting themes
// ---------------------------------------------------------------------------
export const PIERRE_THEME = {
  dark: "github-dark-high-contrast" as const,
  light: "github-light" as const,
};
