"use client";

import { useTranslation } from "react-i18next";
import {
  SettingsTabs,
  SettingsTabsList,
  SettingsTabsPanel,
  type SettingsTabOption,
} from "@/components/settings/settings-tabs";
import { useSettingsTab } from "@/hooks/domains/settings/use-settings-tab";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import { RetentionSettingsCard } from "./retention-settings-card";
import { StorageMaintenanceSettings } from "./storage/storage-maintenance-settings";
import { SystemRouteShell } from "./system-route-shell";

const STORAGE_TARGET_TO_TAB = {
  [SYSTEM_SETTINGS_TARGETS.storageActions]: "host",
  [SYSTEM_SETTINGS_TARGETS.storageSchedule]: "host",
  [SYSTEM_SETTINGS_TARGETS.storageWorkspaces]: "host",
  [SYSTEM_SETTINGS_TARGETS.storageGoCache]: "host",
  [SYSTEM_SETTINGS_TARGETS.storageDocker]: "host",
  [SYSTEM_SETTINGS_TARGETS.storageQuarantine]: "host",
  [SYSTEM_SETTINGS_TARGETS.retention]: "office-retention",
} as const;

export function StorageSettings() {
  const { t } = useTranslation();
  const tabs: SettingsTabOption[] = [
    { id: "host", label: t("system:storageHostTab") },
    { id: "office-retention", label: t("system:officeRetentionTab") },
  ];
  const { value, selectTab } = useSettingsTab({
    tabs: tabs.map((tab) => tab.id),
    defaultTab: "host",
    targetToTab: STORAGE_TARGET_TO_TAB,
  });

  return (
    <SettingsTabs tabs={tabs} value={value} onValueChange={selectTab}>
      <SystemRouteShell
        titleKey="system:storageTitle"
        descriptionKey="system:storageDescription"
        tabs={<SettingsTabsList ariaLabel={t("system:storageTitle")} />}
      >
        <SettingsTabsPanel value="host" testId="settings-storage-host">
          <StorageMaintenanceSettings />
        </SettingsTabsPanel>
        <SettingsTabsPanel value="office-retention" testId="settings-storage-office-retention">
          <RetentionSettingsCard />
        </SettingsTabsPanel>
      </SystemRouteShell>
    </SettingsTabs>
  );
}
