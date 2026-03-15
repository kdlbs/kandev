import { SHORTCUTS, type KeyboardShortcut } from "./constants";

export type ConfigurableShortcutId = "SEARCH" | "FILE_SEARCH" | "QUICK_CHAT";

export type StoredShortcutOverrides = Record<
  string,
  { key: string; modifiers?: Record<string, boolean> }
>;

export const CONFIGURABLE_SHORTCUTS: Record<
  ConfigurableShortcutId,
  { label: string; default: KeyboardShortcut }
> = {
  SEARCH: { label: "Command Panel", default: SHORTCUTS.SEARCH },
  FILE_SEARCH: { label: "File Search", default: SHORTCUTS.FILE_SEARCH },
  QUICK_CHAT: { label: "Quick Chat", default: SHORTCUTS.QUICK_CHAT },
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
  return {
    SEARCH: getShortcut("SEARCH", overrides),
    FILE_SEARCH: getShortcut("FILE_SEARCH", overrides),
    QUICK_CHAT: getShortcut("QUICK_CHAT", overrides),
  };
}
