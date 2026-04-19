/** ANSI escape / control sequences for common terminal keys. */
export const KEY_SEQUENCES = {
  esc: "\x1b",
  tab: "\t",
  up: "\x1b[A",
  down: "\x1b[B",
  right: "\x1b[C",
  left: "\x1b[D",
  home: "\x01",
  end: "\x05",
  pageUp: "\x1b[5~",
  pageDown: "\x1b[6~",
  ctrlC: "\x03",
  ctrlD: "\x04",
  ctrlZ: "\x1a",
} as const;

/**
 * Return the Ctrl+<letter> control byte for a given character (A–Z, a–z).
 * Non-letters return null — callers should then emit the raw char instead.
 */
export function ctrlChord(char: string): string | null {
  if (char.length !== 1) return null;
  const code = char.toLowerCase().charCodeAt(0);
  if (code < 97 || code > 122) return null;
  return String.fromCharCode(code - 96);
}
