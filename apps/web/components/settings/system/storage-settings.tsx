"use client";

import { useTranslation } from "react-i18next";
import {
  SettingsTabs,
  SettingsTabsList,
  SettingsTabsPanel,
  type SettingsTabOption,
} from "@/components/settings/settings-tabs";
import { useFeature } from "@/hooks/domains/features/use-feature";
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
} as const;

const OFFICE_STORAGE_TARGET_TO_TAB = {
  ...STORAGE_TARGET_TO_TAB,
  [SYSTEM_SETTINGS_TARGETS.retention]: "office-retention",
} as const;

export function StorageSettings() {
  const { t } = useTranslation();
  const officeEnabled = useFeature("office");
  const tabs: SettingsTabOption[] = [{ id: "host", label: t("system:storageHostTab") }];
  if (officeEnabled) tabs.push({ id: "office-retention", label: t("system:officeRetentionTab") });
  const { value, selectTab } = useSettingsTab({
    tabs: tabs.map((tab) => tab.id),
    defaultTab: "host",
    targetToTab: officeEnabled ? OFFICE_STORAGE_TARGET_TO_TAB : STORAGE_TARGET_TO_TAB,
  });

  return (
    <SettingsTabs tabs={tabs} value={value} onValueChange={selectTab}>
      <SystemRouteShell
        titleKey="system:storageTitle"
        descriptionKey="system:storageDescription"
        tabs={<SettingsTabsList ariaLabel={t("system:storageTitle")} />}
      >
        <SettingsTabsPanel value="host" testId="settings-storage-host">
          <StorageMaintenanceSettings active={value === "host"} />
        </SettingsTabsPanel>
        {officeEnabled && (
          <SettingsTabsPanel value="office-retention" testId="settings-storage-office-retention">
            <RetentionSettingsCard />
          </SettingsTabsPanel>
        )}
      </SystemRouteShell>
    </SettingsTabs>
  );
}
