import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";

/**
 * How the settings menu renders — a per-device preference.
 *
 * Deliberately *not* part of user settings: the menu belongs to the surface
 * you are looking at, not to the account. A phone reaching the same install
 * wants the flat list even when the desktop beside it wants the tree, and the
 * value has to be readable before the store hydrates (the sidebar paints the
 * menu on the first frame — a mode arriving one render later is a visible
 * flip). Both rule out `updateUserSettings`, which is async and account-wide.
 *
 * `flat` is the default and is exactly the two-level menu
 * (`settings-menu-sections.ts`) with no branches rendered at all.
 */
export const SETTINGS_MENU_MODES = ["flat", "accordion", "persistent"] as const;

export type SettingsMenuMode = (typeof SETTINGS_MENU_MODES)[number];

export const DEFAULT_SETTINGS_MENU_MODE: SettingsMenuMode = "flat";

/** True when `mode` renders branches at all — i.e. anything but `flat`. */
export function isTreeSettingsMenuMode(mode: SettingsMenuMode): boolean {
  return mode !== "flat";
}

function isSettingsMenuMode(value: unknown): value is SettingsMenuMode {
  return SETTINGS_MENU_MODES.includes(value as SettingsMenuMode);
}

/**
 * The stored value, or the default. Re-validated rather than trusted: a mode
 * removed in an upgrade would otherwise leave the menu rendering nothing.
 */
export function readSettingsMenuMode(): SettingsMenuMode {
  const stored = getLocalStorage<string>(
    STORAGE_KEYS.SETTINGS_MENU_MODE,
    DEFAULT_SETTINGS_MENU_MODE,
  );
  return isSettingsMenuMode(stored) ? stored : DEFAULT_SETTINGS_MENU_MODE;
}

export function writeSettingsMenuMode(mode: SettingsMenuMode): void {
  setLocalStorage(STORAGE_KEYS.SETTINGS_MENU_MODE, mode);
}

/**
 * Branch keys the persistent tree should reopen on this device.
 *
 * Keys are node identities, not routes, so a key naming a workspace, agent or
 * executor that has since been deleted simply never matches a rendered branch
 * — stale entries are inert rather than wrong, and get dropped the next time
 * the set is written.
 */
export function readSettingsMenuExpandedKeys(): string[] {
  const stored = getLocalStorage<string[]>(STORAGE_KEYS.SETTINGS_MENU_EXPANDED, []) as unknown;
  if (!Array.isArray(stored)) return [];
  return stored.filter((key): key is string => typeof key === "string");
}

export function writeSettingsMenuExpandedKeys(keys: readonly string[]): void {
  setLocalStorage(STORAGE_KEYS.SETTINGS_MENU_EXPANDED, [...keys]);
}
