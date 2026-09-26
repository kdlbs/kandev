import type { SettingsDiscoveryDefinition } from "../types";
import { GENERAL_SETTINGS_TARGETS } from "./preferences";

export const EXECUTORS_SETTINGS_HREF = "/settings/executors";

export const EXECUTOR_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "executors",
    kind: "page",
    labelKey: "common:executors",
    aliasesKey: "common:commandExecutorsSettingsKeywords",
    groupId: "agents",
    href: EXECUTORS_SETTINGS_HREF,
    order: 400,
  },
  // Sprites.dev config lives on the Executors page: it configures that backend.
  {
    id: "sprites-connection",
    kind: "section",
    labelKey: "settings:connection",
    parentId: "executors",
    groupId: "agents",
    href: EXECUTORS_SETTINGS_HREF,
    targetId: GENERAL_SETTINGS_TARGETS.spritesConnection,
    order: 410,
  },
  {
    id: "sprites-instances",
    kind: "section",
    labelKey: "settings:runningSprites",
    parentId: "executors",
    groupId: "agents",
    href: EXECUTORS_SETTINGS_HREF,
    targetId: GENERAL_SETTINGS_TARGETS.spritesInstances,
    order: 411,
  },
];
