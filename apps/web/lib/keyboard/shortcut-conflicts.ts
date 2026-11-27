/** Direct accepted-event overlaps between effective shortcuts used by settings warnings. */
import type { KeyboardShortcut } from "./constants";
import { isUnboundShortcut } from "./shortcut-overrides";
import type { ShortcutEntry } from "./plugin-shortcuts";
import { matchesShortcut } from "./utils";

/** Two entries sharing at least one accepted event; overlap is not transitive. */
export interface ShortcutConflictGroup {
  entries: ShortcutEntry[];
}

function shortcutsOverlap(first: KeyboardShortcut, second: KeyboardShortcut): boolean {
  if (isUnboundShortcut(first) || isUnboundShortcut(second)) return false;
  if (first.key.toLowerCase() !== second.key.toLowerCase()) return false;

  // Every accepted modifier state is evaluated by the dispatch predicate.
  for (let mask = 0; mask < 16; mask++) {
    const event = {
      key: first.key,
      ctrlKey: Boolean(mask & 1),
      metaKey: Boolean(mask & 2),
      altKey: Boolean(mask & 4),
      shiftKey: Boolean(mask & 8),
    } as KeyboardEvent;
    if (matchesShortcut(event, first) && matchesShortcut(event, second)) return true;
  }
  return false;
}

/** Returns each directly overlapping pair once in input order. */
export function findShortcutConflicts(
  resolved: { entry: ShortcutEntry; shortcut: KeyboardShortcut }[],
): ShortcutConflictGroup[] {
  const groups: ShortcutConflictGroup[] = [];
  for (let left = 0; left < resolved.length; left++) {
    for (let right = left + 1; right < resolved.length; right++) {
      if (shortcutsOverlap(resolved[left].shortcut, resolved[right].shortcut)) {
        groups.push({ entries: [resolved[left].entry, resolved[right].entry] });
      }
    }
  }
  return groups;
}
