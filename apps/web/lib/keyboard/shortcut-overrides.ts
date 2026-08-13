import { SHORTCUTS, type KeyboardShortcut } from "./constants";

export type ConfigurableShortcutId =
  | "SEARCH"
  | "FILE_SEARCH"
  | "CONTENT_SEARCH"
  | "QUICK_CHAT"
  | "BOTTOM_TERMINAL"
  | "TOGGLE_SIDEBAR"
  | "COMMAND_PANEL"
  | "NEW_TASK"
  | "FOCUS_INPUT"
  | "FOCUS_PASSTHROUGH_INPUT"
  | "TOGGLE_PLAN_MODE"
  | "TASK_SWITCHER"
  | "TASK_SWITCHER_REVERSE"
  | "REVERSE_SEARCH"
  | "OPEN_TASK_PR"
  | "WORKSPACE_PICKER";

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
  { labelKey: string; default: KeyboardShortcut }
> = {
  SEARCH: { labelKey: "settings:shortcutCommandPanel", default: SHORTCUTS.SEARCH },
  FILE_SEARCH: { labelKey: "settings:shortcutFileSearch", default: SHORTCUTS.FILE_SEARCH },
  CONTENT_SEARCH: {
    labelKey: "settings:shortcutTaskContentSearch",
    default: SHORTCUTS.CONTENT_SEARCH,
  },
  QUICK_CHAT: { labelKey: "settings:shortcutQuickChat", default: SHORTCUTS.QUICK_CHAT },
  BOTTOM_TERMINAL: {
    labelKey: "settings:shortcutBottomTerminal",
    default: SHORTCUTS.BOTTOM_TERMINAL,
  },
  TOGGLE_SIDEBAR: { labelKey: "settings:shortcutToggleSidebar", default: UNBOUND_SHORTCUT },
  COMMAND_PANEL: { labelKey: "settings:shortcutCommandPanelAlt", default: SHORTCUTS.COMMAND_PANEL },
  NEW_TASK: { labelKey: "settings:shortcutNewTask", default: SHORTCUTS.NEW_TASK },
  FOCUS_INPUT: { labelKey: "settings:shortcutFocusChatInput", default: SHORTCUTS.FOCUS_INPUT },
  FOCUS_PASSTHROUGH_INPUT: {
    labelKey: "settings:shortcutFocusCliChatInput",
    default: SHORTCUTS.FOCUS_PASSTHROUGH_INPUT,
  },
  TOGGLE_PLAN_MODE: {
    labelKey: "settings:shortcutTogglePlanMode",
    default: SHORTCUTS.TOGGLE_PLAN_MODE,
  },
  TASK_SWITCHER: {
    labelKey: "settings:shortcutRecentTaskSwitcher",
    default: SHORTCUTS.TASK_SWITCHER,
  },
  TASK_SWITCHER_REVERSE: {
    labelKey: "settings:shortcutRecentTaskSwitcherBackward",
    default: SHORTCUTS.TASK_SWITCHER_REVERSE,
  },
  REVERSE_SEARCH: {
    labelKey: "settings:shortcutReverseChatSearch",
    default: SHORTCUTS.REVERSE_SEARCH,
  },
  OPEN_TASK_PR: {
    labelKey: "settings:shortcutOpenTaskPullRequest",
    default: SHORTCUTS.OPEN_TASK_PR,
  },
  WORKSPACE_PICKER: {
    labelKey: "settings:shortcutOpenWorkspacePicker",
    default: SHORTCUTS.WORKSPACE_PICKER,
  },
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
