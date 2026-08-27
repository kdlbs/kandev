"use client";

import { Separator } from "@kandev/ui/separator";
import { useTranslation } from "react-i18next";
import { SettingsTarget } from "@/components/settings/settings-target";
import { BackupsTable } from "@/components/settings/system/backups-table";
import { DatabaseStatsCard } from "@/components/settings/system/database-stats-card";
import { LogViewer } from "@/components/settings/system/log-viewer";
import { StorageMaintenanceSettings } from "@/components/settings/system/storage/storage-maintenance-settings";
import { BACKUP_DIR, BACKUP_SQL_COMMAND } from "@/components/settings/system/system-route-shell";
import { SYSTEM_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/system";

function SectionHeading({ title, description }: { title: string; description?: string }) {
  return (
    <div>
      <h3 className="text-lg font-semibold">{title}</h3>
      {description && <p className="text-sm text-muted-foreground mt-1">{description}</p>}
    </div>
  );
}

/** Data & storage: the former Database, Backups, Storage and Logs pages as one page. */
export function DataStorageSettings() {
  const { t } = useTranslation();
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
      <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.backups} className="space-y-4">
        <SectionHeading
          title={t("system:navBackups")}
          description={t("system:backupsPageDescription", {
            command: BACKUP_SQL_COMMAND,
            path: BACKUP_DIR,
          })}
        />
        <BackupsTable />
      </SettingsTarget>
      <Separator />
      <div className="space-y-4">
        <SectionHeading
          title={t("system:storageTitle")}
          description={t("system:storageDescription")}
        />
        <StorageMaintenanceSettings />
      </div>
      <Separator />
      <SettingsTarget targetId={SYSTEM_SETTINGS_TARGETS.logs} className="space-y-4">
        <SectionHeading
          title={t("system:navLogs")}
          description={t("settings:logsPageDescription")}
        />
        <LogViewer />
      </SettingsTarget>
    </div>
  );
}
