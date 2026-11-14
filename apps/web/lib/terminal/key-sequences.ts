/**
 * ANSI escape / control sequences for common terminal keys.
 *
 * Note: `home` / `end` send readline shortcuts (Ctrl+A / Ctrl+E) rather than
 * the VT sequences (`\x1b[H` / `\x1b[F`). This matches what most shells expect
 * for line-start / line-end navigation; full-screen apps like vim/nano that
 * key off VT sequences will not interpret these as Home/End.
 */
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

export type ArrowKey = "up" | "down" | "left" | "right";

export function isArrowKey(key: string): key is ArrowKey {
  return key === "up" || key === "down" || key === "left" || key === "right";
}

export function arrowSequence(key: ArrowKey, application: boolean): string {
  const normal = KEY_SEQUENCES[key];
  return application ? `\x1bO${normal.slice(2)}` : normal;
}
