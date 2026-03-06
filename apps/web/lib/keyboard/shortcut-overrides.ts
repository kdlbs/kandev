import { SHORTCUTS, type KeyboardShortcut } from "./constants";

const STORAGE_KEY = "kandev.keyboard-shortcuts";

export type ConfigurableShortcutId = "SEARCH" | "FILE_SEARCH";

export const CONFIGURABLE_SHORTCUTS: Record<
  ConfigurableShortcutId,
  { label: string; default: KeyboardShortcut }
> = {
  SEARCH: { label: "Command Panel", default: SHORTCUTS.SEARCH },
  FILE_SEARCH: { label: "File Search", default: SHORTCUTS.FILE_SEARCH },
};

type StoredOverrides = Partial<Record<ConfigurableShortcutId, KeyboardShortcut>>;

function loadOverrides(): StoredOverrides {
  if (typeof window === "undefined") return {};
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    return JSON.parse(raw) as StoredOverrides;
  } catch {
    return {};
  }
}

function saveOverrides(overrides: StoredOverrides): void {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(overrides));
}

export function getShortcut(id: ConfigurableShortcutId): KeyboardShortcut {
  const overrides = loadOverrides();
  return overrides[id] ?? CONFIGURABLE_SHORTCUTS[id].default;
}

export function setShortcut(id: ConfigurableShortcutId, shortcut: KeyboardShortcut): void {
  const overrides = loadOverrides();
  overrides[id] = shortcut;
  saveOverrides(overrides);
}

export function resetShortcut(id: ConfigurableShortcutId): void {
  const overrides = loadOverrides();
  delete overrides[id];
  saveOverrides(overrides);
}

export function getAllConfigurableShortcuts(): Record<ConfigurableShortcutId, KeyboardShortcut> {
  const overrides = loadOverrides();
  return {
    SEARCH: overrides.SEARCH ?? CONFIGURABLE_SHORTCUTS.SEARCH.default,
    FILE_SEARCH: overrides.FILE_SEARCH ?? CONFIGURABLE_SHORTCUTS.FILE_SEARCH.default,
  };
}
