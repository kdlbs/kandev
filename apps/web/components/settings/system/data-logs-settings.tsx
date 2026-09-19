"use client";

import { useEffect, useState } from "react";
import { Separator } from "@kandev/ui/separator";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { SettingsTarget } from "@/components/settings/settings-target";
import {
  SettingsTabs,
  SettingsTabsList,
  SettingsTabsPanel,
  type SettingsTabOption,
} from "@/components/settings/settings-tabs";
import { BackupsTable } from "@/components/settings/system/backups-table";
import { DatabaseStatsCard } from "@/components/settings/system/database-stats-card";
import { LogViewer } from "@/components/settings/system/log-viewer";
import { useSettingsTab } from "@/hooks/domains/settings/use-settings-tab";
import { useRouter } from "@/lib/routing/client-router";
import { settingsTargetFromHash } from "@/lib/settings-discovery/target";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";
import { ToolPayloadRetentionCard } from "./tool-payload-retention-card";
import { BACKUP_SQL_COMMAND, SystemRouteShell } from "./system-route-shell";

function SectionHeading({ title, description }: { title: string; description?: string }) {
  return (
    <div>
      <h3 className="text-lg font-semibold">{title}</h3>
      {description && (
        <p className="mt-1 break-words text-sm text-muted-foreground">{description}</p>
      )}
    </div>
  );
}

const DATA_LOGS_TARGET_TO_TAB = {
  [SYSTEM_SETTINGS_TARGETS.database]: "database",
  [SYSTEM_SETTINGS_TARGETS.toolPayloadRetention]: "database",
  [SYSTEM_SETTINGS_TARGETS.backups]: "database",
  [SYSTEM_SETTINGS_TARGETS.logs]: "logs",
} as const;

function DatabasePanel() {
  const { t } = useTranslation();
  const backupDirectory = useAppStore((s) => s.system.database?.backup_directory);
  return (
    <div className="space-y-8">
      <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.database} className="space-y-4">
        <SectionHeading
          title={t("system:navDatabase")}
          description={t("system:databasePageDescription")}
        />
        <DatabaseStatsCard />
      </SettingsTarget>
      <Separator />
      <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.toolPayloadRetention}>
        <ToolPayloadRetentionCard />
      </SettingsTarget>
      <Separator />
      <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.backups} className="space-y-4">
        <SectionHeading
          title={t("system:navBackups")}
          description={
            backupDirectory
              ? t("system:backupsPageDescription", {
                  command: BACKUP_SQL_COMMAND,
                  path: backupDirectory,
                })
              : undefined
          }
        />
        <BackupsTable />
      </SettingsTarget>
    </div>
  );
}

function LogsPanel() {
  const { t } = useTranslation();
  return (
    <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.logs} className="space-y-4">
      <SectionHeading title={t("system:navLogs")} description={t("settings:logsPageDescription")} />
      <LogViewer />
    </SettingsTarget>
  );
}

export function DataLogsSettings() {
  const { t } = useTranslation();
  const router = useRouter();
  const [locationHash, setLocationHash] = useState(() =>
    typeof window === "undefined" ? "" : window.location.hash,
  );
  const tabs: SettingsTabOption[] = [
    { id: "database", label: t("system:navDatabase") },
    { id: "logs", label: t("system:navLogs") },
  ];
  const { value, selectTab } = useSettingsTab({
    tabs: tabs.map((tab) => tab.id),
    defaultTab: "database",
    targetToTab: DATA_LOGS_TARGET_TO_TAB,
  });

  useEffect(() => {
    const updateHash = () => setLocationHash(window.location.hash);
    window.addEventListener("hashchange", updateHash);
    window.addEventListener("popstate", updateHash);
    return () => {
      window.removeEventListener("hashchange", updateHash);
      window.removeEventListener("popstate", updateHash);
    };
  }, []);

  useEffect(() => {
    const targetId = settingsTargetFromHash(locationHash);
    if (targetId !== SYSTEM_SETTINGS_TARGETS.retention) return;
    const params = new URLSearchParams(window.location.search);
    params.set("tab", "office-retention");
    router.replace(`/settings/system/storage?${params.toString()}${locationHash}`, {
      scroll: false,
    });
  }, [locationHash, router]);

  return (
    <SettingsTabs tabs={tabs} value={value} onValueChange={selectTab}>
      <SystemRouteShell
        titleKey="system:navDataStorage"
        descriptionKey="system:dataStoragePageDescription"
        tabs={<SettingsTabsList ariaLabel={t("system:navDataStorage")} />}
      >
        <SettingsTabsPanel value="database" testId="settings-data-storage-database">
          <DatabasePanel />
        </SettingsTabsPanel>
        <SettingsTabsPanel value="logs" testId="settings-data-storage-logs">
          <LogsPanel />
        </SettingsTabsPanel>
      </SystemRouteShell>
    </SettingsTabs>
  );
}
