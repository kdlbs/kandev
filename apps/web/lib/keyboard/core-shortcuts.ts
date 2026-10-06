import { SHORTCUTS } from "./constants";

// Editor save and panel search are reserved independently of configurable shortcuts.
export const NON_CONFIGURABLE_CORE_SHORTCUT_IDS = [
  "FIND_IN_PANEL",
  "SAVE",
] as const satisfies ReadonlyArray<keyof typeof SHORTCUTS>;
