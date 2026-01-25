import type { ITheme } from '@xterm/xterm';

/**
 * Shared xterm.js theme configuration for all terminal components.
 * Uses transparent background so terminals inherit from their container's bg-background.
 */
export const terminalTheme: ITheme = {
  background: 'transparent',
  foreground: '#d4d4d4',
  cursor: '#d4d4d4',
  cursorAccent: 'transparent',
  selectionBackground: '#264f78',
  black: '#1e1e1e',
  red: '#f44747',
  green: '#6a9955',
  yellow: '#dcdcaa',
  blue: '#569cd6',
  magenta: '#c586c0',
  cyan: '#4ec9b0',
  white: '#d4d4d4',
  brightBlack: '#808080',
  brightRed: '#f44747',
  brightGreen: '#6a9955',
  brightYellow: '#dcdcaa',
  brightBlue: '#569cd6',
  brightMagenta: '#c586c0',
  brightCyan: '#4ec9b0',
  brightWhite: '#ffffff',
};

/**
 * Apply transparent background to xterm.js internal elements.
 * This ensures the terminal inherits the container's background color.
 */
export function applyTransparentBackground(container: HTMLElement): void {
  const selectors = ['.xterm', '.xterm-viewport', '.xterm-screen', '.xterm-scrollable-element'];
  selectors.forEach((selector) => {
    const el = container.querySelector(selector) as HTMLElement | null;
    if (el) {
      el.style.background = 'transparent';
      el.style.backgroundColor = 'transparent';
    }
  });
}
