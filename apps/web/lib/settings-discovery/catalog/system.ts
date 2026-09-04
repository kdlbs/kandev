import type { SettingsDiscoveryDefinition } from "../types";

export const SYSTEM_SETTINGS_HREF = "/settings/system";
export const SYSTEM_STATUS_SETTINGS_HREF = `${SYSTEM_SETTINGS_HREF}/status`;
const SYSTEM_DATA_STORAGE_DISCOVERY_ID = "system-data-storage";
const SYSTEM_STORAGE_DISCOVERY_ID = "system-storage";
export const SYSTEM_DATA_STORAGE_SETTINGS_HREF = `${SYSTEM_SETTINGS_HREF}/data-storage`;
export const SYSTEM_STORAGE_SETTINGS_HREF = `${SYSTEM_SETTINGS_HREF}/storage`;
export const SYSTEM_ABOUT_SETTINGS_HREF = `${SYSTEM_SETTINGS_HREF}/about`;
export const SYSTEM_SETTINGS_TARGETS = {
  database: "setting-system-database",
  backups: "setting-system-backups",
  logs: "setting-system-logs",
  licenses: "setting-system-licenses",
  storageActions: "setting-system-storage-actions",
  storageSchedule: "setting-system-storage-schedule",
  storageWorkspaces: "setting-system-storage-workspaces",
  storageGoCache: "setting-system-storage-go-cache",
  storageDocker: "setting-system-storage-docker",
  storageQuarantine: "setting-system-storage-quarantine",
} as const;

const STORAGE_SECTION_TARGETS: Record<string, string> = {
  schedule: SYSTEM_SETTINGS_TARGETS.storageSchedule,
  workspaces: SYSTEM_SETTINGS_TARGETS.storageWorkspaces,
  "go-cache": SYSTEM_SETTINGS_TARGETS.storageGoCache,
  docker: SYSTEM_SETTINGS_TARGETS.storageDocker,
  quarantine: SYSTEM_SETTINGS_TARGETS.storageQuarantine,
};

export function storageSettingsTarget(sectionId: string): string | undefined {
  return STORAGE_SECTION_TARGETS[sectionId];
}

export const SYSTEM_DISCOVERY_DEFINITIONS: SettingsDiscoveryDefinition[] = [
  {
    id: "system-status",
    kind: "page",
    labelKey: "common:status",
    groupId: "system",
    href: SYSTEM_STATUS_SETTINGS_HREF,
    order: 610,
  },
  {
    id: SYSTEM_DATA_STORAGE_DISCOVERY_ID,
    kind: "page",
    labelKey: "system:navDataStorage",
    groupId: "system",
    href: SYSTEM_DATA_STORAGE_SETTINGS_HREF,
    order: 620,
  },
  {
    id: "system-database",
    kind: "section",
    labelKey: "system:navDatabase",
    parentId: SYSTEM_DATA_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_DATA_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.database,
    order: 621,
  },
  {
    id: "system-backups",
    kind: "section",
    labelKey: "system:navBackups",
    parentId: SYSTEM_DATA_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_DATA_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.backups,
    order: 622,
  },
  {
    id: "system-logs",
    kind: "section",
    labelKey: "system:navLogs",
    parentId: SYSTEM_DATA_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_DATA_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.logs,
    order: 623,
  },
  {
    id: "system-storage",
    kind: "page",
    labelKey: "system:storageTitle",
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    order: 625,
  },
  {
    id: "system-storage-actions",
    kind: "section",
    labelKey: "system:storageActionsTitle",
    parentId: SYSTEM_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.storageActions,
    order: 626,
  },
  {
    id: "system-storage-schedule",
    kind: "section",
    labelKey: "system:storageScheduleTitle",
    parentId: SYSTEM_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.storageSchedule,
    order: 627,
  },
  {
    id: "system-storage-workspaces",
    kind: "section",
    labelKey: "system:storageWorkspacesTitle",
    parentId: SYSTEM_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.storageWorkspaces,
    order: 628,
  },
  {
    id: "system-storage-go-cache",
    kind: "section",
    labelKey: "system:storageGoBuildCache",
    parentId: SYSTEM_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.storageGoCache,
    order: 629,
  },
  {
    id: "system-storage-docker",
    kind: "section",
    labelKey: "system:storageDockerTitle",
    parentId: SYSTEM_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.storageDocker,
    order: 630,
  },
  {
    id: "system-storage-quarantine",
    kind: "section",
    labelKey: "system:storageQuarantineSafetyTitle",
    parentId: SYSTEM_STORAGE_DISCOVERY_ID,
    groupId: "system",
    href: SYSTEM_STORAGE_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.storageQuarantine,
    order: 631,
  },
  {
    id: "system-feature-toggles",
    kind: "page",
    labelKey: "system:navFeatureToggles",
    groupId: "system",
    href: "/settings/system/feature-toggles",
    order: 640,
  },
  {
    id: "system-updates",
    kind: "page",
    labelKey: "system:navUpdates",
    groupId: "system",
    href: "/settings/system/updates",
    order: 650,
  },
  {
    id: "system-about",
    kind: "page",
    labelKey: "system:navAbout",
    groupId: "system",
    href: SYSTEM_ABOUT_SETTINGS_HREF,
    order: 660,
  },
  {
    id: "system-licenses",
    kind: "section",
    labelKey: "system:navLicenses",
    parentId: "system-about",
    groupId: "system",
    href: SYSTEM_ABOUT_SETTINGS_HREF,
    targetId: SYSTEM_SETTINGS_TARGETS.licenses,
    order: 661,
  },
  {
    id: "system-users",
    kind: "page",
    labelKey: "system:navUsers",
    groupId: "access",
    href: "/settings/system/users",
    order: 670,
    requires: "users",
  },
  // Grouped with access control, not system: an organization is a boundary
  // above users, and System is for operating the instance.
  {
    id: "settings-units",
    kind: "page",
    labelKey: "settings:navUnits",
    groupId: "access",
    href: "/settings/units",
    order: 659,
    requires: "users",
  },
  {
    id: "system-organizations",
    kind: "page",
    labelKey: "orgs:navOrganizations",
    groupId: "access",
    href: "/settings/system/organizations",
    order: 661,
    requires: "organizations",
  },
];
