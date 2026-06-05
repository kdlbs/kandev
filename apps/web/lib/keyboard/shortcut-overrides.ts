import { SHORTCUTS, type KeyboardShortcut } from "./constants";

export type ConfigurableShortcutId =
  | "SEARCH"
  | "FILE_SEARCH"
  | "QUICK_CHAT"
  | "BOTTOM_TERMINAL"
  | "TOGGLE_SIDEBAR"
  | "COMMAND_PANEL"
  | "NEW_TASK"
  | "FOCUS_INPUT"
  | "TOGGLE_PLAN_MODE"
  | "TASK_SWITCHER"
  | "TASK_SWITCHER_REVERSE"
  | "VOICE_INPUT_TOGGLE"
  | "REVERSE_SEARCH";

export type StoredShortcutOverrides = Record<
  string,
  { key: string; modifiers?: Record<string, boolean> }
>;

/**
 * Sentinel "no shortcut" value. `matchesShortcut` never matches a real key
 * event against an empty key, so using this as a default makes a shortcut
 * unbound until the user records one.
 */
export const UNBOUND_SHORTCUT: KeyboardShortcut = { key: "" as KeyboardShortcut["key"] };

export function isUnboundShortcut(shortcut: KeyboardShortcut | undefined | null): boolean {
  return !shortcut || (shortcut.key as string) === "";
}

export const CONFIGURABLE_SHORTCUTS: Record<
  ConfigurableShortcutId,
  { label: string; default: KeyboardShortcut }
> = {
  SEARCH: { label: "Command Panel", default: SHORTCUTS.SEARCH },
  FILE_SEARCH: { label: "File Search", default: SHORTCUTS.FILE_SEARCH },
  QUICK_CHAT: { label: "Quick Chat", default: SHORTCUTS.QUICK_CHAT },
  BOTTOM_TERMINAL: { label: "Toggle Bottom Terminal", default: SHORTCUTS.BOTTOM_TERMINAL },
  TOGGLE_SIDEBAR: { label: "Toggle Sidebar", default: UNBOUND_SHORTCUT },
  COMMAND_PANEL: { label: "Command Panel (Alt)", default: SHORTCUTS.COMMAND_PANEL },
  NEW_TASK: { label: "New Task", default: SHORTCUTS.NEW_TASK },
  FOCUS_INPUT: { label: "Focus Chat Input", default: SHORTCUTS.FOCUS_INPUT },
  TOGGLE_PLAN_MODE: { label: "Toggle Plan Mode", default: SHORTCUTS.TOGGLE_PLAN_MODE },
  TASK_SWITCHER: { label: "Recent Task Switcher", default: SHORTCUTS.TASK_SWITCHER },
  TASK_SWITCHER_REVERSE: {
    label: "Recent Task Switcher (Backward)",
    default: SHORTCUTS.TASK_SWITCHER_REVERSE,
  },
  VOICE_INPUT_TOGGLE: { label: "Voice Input", default: SHORTCUTS.VOICE_INPUT_TOGGLE },
  REVERSE_SEARCH: { label: "Reverse Chat Search", default: SHORTCUTS.REVERSE_SEARCH },
};

export function getShortcut(
  id: ConfigurableShortcutId,
  overrides?: StoredShortcutOverrides,
): KeyboardShortcut {
  const override = overrides?.[id];
  if (override) return override as KeyboardShortcut;
  return CONFIGURABLE_SHORTCUTS[id].default;
}

export function resolveAllShortcuts(
  overrides?: StoredShortcutOverrides,
): Record<ConfigurableShortcutId, KeyboardShortcut> {
  const ids = Object.keys(CONFIGURABLE_SHORTCUTS) as ConfigurableShortcutId[];
  const result = {} as Record<ConfigurableShortcutId, KeyboardShortcut>;
  for (const id of ids) {
    result[id] = getShortcut(id, overrides);
  }
  return result;
}
