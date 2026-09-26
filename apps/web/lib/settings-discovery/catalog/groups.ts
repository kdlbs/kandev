import type { SettingsDiscoveryGroupDefinition } from "../types";

export const SETTINGS_DISCOVERY_GROUPS: SettingsDiscoveryGroupDefinition[] = [
  { id: "preferences", labelKey: "settings:preferences", order: 0 },
  { id: "workspaces", labelKey: "settings:workspacesAndAccess", order: 1 },
  { id: "agents", labelKey: "common:agents", order: 2 },
  { id: "access", labelKey: "settings:accessControl", order: 3 },
  { id: "system", labelKey: "common:system", order: 4 },
];
