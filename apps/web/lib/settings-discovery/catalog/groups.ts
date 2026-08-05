import type { SettingsDiscoveryGroupDefinition } from "../types";

export const SETTINGS_DISCOVERY_GROUPS: SettingsDiscoveryGroupDefinition[] = [
  { id: "general", labelKey: "settings:general", order: 0 },
  { id: "workspaces", labelKey: "common:workspaces", order: 1 },
  { id: "agents", labelKey: "common:agents", order: 2 },
  { id: "executors", labelKey: "common:executors", order: 3 },
  { id: "tools", labelKey: "common:settings", order: 4 },
  { id: "system", labelKey: "common:system", order: 5 },
  { id: "account", labelKey: "sidebar:account", order: 6 },
];
