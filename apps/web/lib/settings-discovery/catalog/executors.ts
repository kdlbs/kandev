import type { SettingsDiscoveryDefinition } from "../types";

export const EXECUTORS_SETTINGS_HREF = "/settings/executors";

export const EXECUTOR_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "executors",
    kind: "page",
    labelKey: "common:executors",
    aliasesKey: "common:commandExecutorsSettingsKeywords",
    groupId: "executors",
    href: EXECUTORS_SETTINGS_HREF,
    order: 400,
  },
];
