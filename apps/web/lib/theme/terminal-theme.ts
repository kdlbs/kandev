import type { ITheme } from "@xterm/xterm";

/**
 * ANSI terminal colors — standard palette, not derived from the app theme.
 * These are used for syntax highlighting, command output, etc.
 */
const ansiColors = {
  black: "#1e1e1e",
  red: "#f44747",
  green: "#6a9955",
  yellow: "#dcdcaa",
  blue: "#569cd6",
  magenta: "#c586c0",
  cyan: "#4ec9b0",
  white: "#d4d4d4",
  brightBlack: "#808080",
  brightRed: "#f44747",
  brightGreen: "#6a9955",
  brightYellow: "#dcdcaa",
  brightBlue: "#569cd6",
  brightMagenta: "#c586c0",
  brightCyan: "#4ec9b0",
  brightWhite: "#ffffff",
} as const;

/**
 * Build the xterm.js theme by resolving CSS custom properties from the DOM.
 *
 * xterm's WebGL addon renders onto a <canvas> so it can't use CSS variables
 * directly — we read computed values at terminal creation time instead.
 *
 * All theme-dependent colors come from CSS variables defined in globals.css,
 * so changing the app theme in one place updates terminals too.
 */
export function getTerminalTheme(container: HTMLElement): ITheme {
  const s = getComputedStyle(container);
  const v = (name: string) => s.getPropertyValue(name).trim();

  return {
    background: v("--background"),
    foreground: v("--foreground"),
    cursor: v("--foreground"),
    cursorAccent: v("--background"),
    selectionBackground: v("--muted"),
    ...ansiColors,
  };
}
