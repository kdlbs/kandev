/**
 * Pure conflict detection over a list of resolved (effective) keyboard
 * shortcuts — used to warn about core-vs-plugin and plugin-vs-plugin combo
 * collisions in Settings > Keyboard Shortcuts. Detection only; UI surfacing
 * is a lightweight per-row indicator (see `keyboard-shortcuts-card.tsx`).
 */
import type { KeyboardShortcut } from "./constants";
import { isUnboundShortcut } from "./shortcut-overrides";
import type { ShortcutEntry } from "./plugin-shortcuts";

/**
 * Canonicalizes a shortcut to a comparable string, resolving `ctrlOrCmd` to
 * the concrete modifier the given platform would fire (mirrors
 * `matchesShortcut`'s runtime behavior). Returns null for an unbound
 * shortcut — unbound entries never conflict with anything.
 */
export function comboKey(shortcut: KeyboardShortcut, isMacPlatform: boolean): string | null {
  if (isUnboundShortcut(shortcut)) return null;

  const modifiers = shortcut.modifiers;
  const ctrl = !!modifiers?.ctrl || !!(modifiers?.ctrlOrCmd && !isMacPlatform);
  const cmd = !!modifiers?.cmd || !!(modifiers?.ctrlOrCmd && isMacPlatform);
  const alt = !!modifiers?.alt;
  const shift = !!modifiers?.shift;

  return [shortcut.key.toLowerCase(), ctrl, cmd, alt, shift].join("|");
}

/** A group of two or more entries whose effective combo collides. */
export interface ShortcutConflictGroup {
  comboKey: string;
  entries: ShortcutEntry[];
}

/**
 * Groups `entries` (already resolved to their effective shortcut, e.g. via
 * `resolveShortcutEntry`) by collision. Only groups with 2+ members are
 * returned. Dispatch order for a colliding combo is registration order (see
 * `hooks/use-plugin-shortcuts.ts`) — this helper only detects and reports,
 * it doesn't resolve precedence.
 */
export function findShortcutConflicts(
  resolved: { entry: ShortcutEntry; shortcut: KeyboardShortcut }[],
  isMacPlatform: boolean,
): ShortcutConflictGroup[] {
  const groups = new Map<string, ShortcutEntry[]>();

  for (const { entry, shortcut } of resolved) {
    const key = comboKey(shortcut, isMacPlatform);
    if (key === null) continue;
    const group = groups.get(key);
    if (group) {
      group.push(entry);
    } else {
      groups.set(key, [entry]);
    }
  }

  return Array.from(groups.entries())
    .filter(([, list]) => list.length > 1)
    .map(([key, list]) => ({ comboKey: key, entries: list }));
}
