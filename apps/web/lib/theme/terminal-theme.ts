import type { ITheme } from "@xterm/xterm";
import type { ResolvedTheme } from "@/components/theme/app-theme";

export const TERMINAL_MINIMUM_CONTRAST_RATIO = 4.5;

/**
 * ANSI terminal colors — standard palette, not derived from the app theme.
 * These are used for syntax highlighting, command output, etc.
 */
const darkAnsiColors = {
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

const lightAnsiColors = {
  black: "#000000",
  red: "#cd3131",
  green: "#008000",
  yellow: "#795e00",
  blue: "#0451a5",
  magenta: "#bc05bc",
  cyan: "#005a5a",
  white: "#555555",
  brightBlack: "#666666",
  brightRed: "#c41a16",
  brightGreen: "#006400",
  brightYellow: "#9c6500",
  brightBlue: "#1769aa",
  brightMagenta: "#9c00a8",
  brightCyan: "#007a7a",
  brightWhite: "#000000",
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
export function getTerminalTheme(container: HTMLElement, resolvedTheme: ResolvedTheme): ITheme {
  const s = getComputedStyle(container);
  const v = (name: string) => s.getPropertyValue(name).trim();
  const ansiColors = resolvedTheme === "light" ? lightAnsiColors : darkAnsiColors;

  return {
    background: v("--background"),
    foreground: v("--foreground"),
    cursor: v("--foreground"),
    cursorAccent: v("--background"),
    selectionBackground: v("--muted"),
    ...ansiColors,
  };
}

/** Build the dark palette used by terminals that are independent of the app theme. */
export function getFixedDarkTerminalTheme(): ITheme {
  return {
    background: "#0b0b0c",
    foreground: "#d4d4d4",
    cursor: "#d4d4d4",
    cursorAccent: "#0b0b0c",
    selectionBackground: "#2a2a2a",
    ...darkAnsiColors,
  };
}
