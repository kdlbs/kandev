import type { StateCreator } from "zustand";
import {
  readSettingsMenuExpandedKeys,
  readSettingsMenuMode,
  writeSettingsMenuExpandedKeys,
  writeSettingsMenuMode,
} from "@/lib/settings/settings-menu-mode";
import type { SettingsMenuState, UISlice } from "./types";

export function loadSettingsMenuState(): SettingsMenuState {
  const mode = readSettingsMenuMode();
  return {
    mode,
    // Nothing is being previewed at boot, so the rendered mode *is* the saved
    // one. They only diverge while the Appearance page holds an unsaved draft.
    savedMode: mode,
    expandedKeys: readSettingsMenuExpandedKeys(),
  };
}

type ImmerSet = Parameters<StateCreator<UISlice, [["zustand/immer", never]], [], UISlice>>[0];

/**
 * Mirrors the theme's preview/commit/restore trio (`components/theme/app-theme`).
 * The settings sidebar is on screen beside the Appearance page, so changing the
 * control repaints the menu immediately; only the shared save coordinator
 * writes to storage, and discarding puts the rendered mode back.
 */
export function buildSettingsMenuActions(set: ImmerSet) {
  return {
    previewSettingsMenuMode: (mode: SettingsMenuState["mode"]) =>
      set((draft) => {
        draft.settingsMenu.mode = mode;
      }),
    commitSettingsMenuMode: (mode: SettingsMenuState["mode"]) =>
      set((draft) => {
        draft.settingsMenu.mode = mode;
        draft.settingsMenu.savedMode = mode;
        writeSettingsMenuMode(mode);
      }),
    restoreSettingsMenuMode: () =>
      set((draft) => {
        draft.settingsMenu.mode = draft.settingsMenu.savedMode;
      }),
    setSettingsMenuExpandedKeys: (keys: string[]) =>
      set((draft) => {
        draft.settingsMenu.expandedKeys = keys;
        writeSettingsMenuExpandedKeys(keys);
      }),
  };
}
