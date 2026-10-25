"use client";

import { IconArrowDown, IconArrowUp, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useOfficeConfigSyncActive } from "@/hooks/domains/office/use-office-config-sync-active";
import { SyncDiffPane } from "./sync-diff-pane";
import { useSyncState } from "./use-sync-state";
import { useTranslation } from "react-i18next";

export function SyncContent() {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const sync = useSyncState(activeWorkspaceId);
  // AC-OFFICE-CONFIG-SYNC-006.6: apply-incoming and apply-outgoing are
  // refused server-side while config sync owns this workspace; the read-only
  // diff views below keep rendering regardless (AC-OFFICE-CONFIG-SYNC-005.3).
  const configSyncActive = useOfficeConfigSyncActive(activeWorkspaceId);
  const applyDisabledReason = configSyncActive
    ? t("office:configSyncActiveGuardReason")
    : undefined;

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-6 py-3 border-b border-border shrink-0">
        <div>
          <h1 className="text-base font-medium">{t("office:sync")}</h1>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t("office:compareWorkspaceDatabaseWithOnDisk")}
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={sync.refresh}
          disabled={sync.loading}
          className="cursor-pointer"
        >
          <IconRefresh className="h-4 w-4 mr-1.5" />
          {t("office:refresh")}
        </Button>
      </div>
      <div className="flex-1 min-h-0 overflow-auto">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 p-6">
          <SyncDiffPane
            title={t("office:incomingFilesystemDatabase")}
            description={t("office:applyOnDiskYamlFilesTo")}
            icon={<IconArrowDown className="h-4 w-4" />}
            diff={sync.incoming}
            loading={sync.loading}
            applying={sync.applyingIn}
            applyLabel={t("office:importFromFilesystem")}
            onApply={sync.applyIncoming}
            disabledReason={applyDisabledReason}
          />
          <SyncDiffPane
            title={t("office:outgoingDatabaseFilesystem")}
            description={t("office:writeTheDatabaseStateToOn")}
            icon={<IconArrowUp className="h-4 w-4" />}
            diff={sync.outgoing}
            loading={sync.loading}
            applying={sync.applyingOut}
            applyLabel={t("office:exportToFilesystem")}
            onApply={sync.applyOutgoing}
            disabledReason={applyDisabledReason}
          />
        </div>
      </div>
    </div>
  );
}
