import { SHORTCUTS } from "./constants";

// These core combinations are reserved independently of configurable shortcuts.
export const NON_CONFIGURABLE_CORE_SHORTCUT_IDS = [
  "FIND_IN_PANEL",
  "SAVE",
  "COMMAND_PANEL_SHIFT",
] as const satisfies ReadonlyArray<keyof typeof SHORTCUTS>;
