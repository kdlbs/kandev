"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { SettingsGroup } from "@/components/settings/settings-group";
import { WorkflowSyncStatusCard } from "@/components/settings/workflow-sync-status-banner";
import { PromptSyncDialog } from "@/components/settings/prompt-sync-dialog";
import { useAppStore } from "@/components/state-provider";
import { usePromptSync } from "@/hooks/domains/settings/use-prompt-sync";

const TEST_ID_PREFIX = "prompt-sync";

// PromptSyncSection is the Prompt Sync entry point on Settings > Prompts: a
// setup prompt when unconfigured, otherwise the shared sync status card with
// Configure and Sync now. Prompt list changes arrive over the WebSocket.
export function PromptSyncSection() {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId) ?? "";
  const sync = usePromptSync(activeWorkspaceId);
  const [open, setOpen] = useState(false);
  const openDialog = () => {
    sync.reset(sync.config);
    setOpen(true);
  };
  const configureButton = (
    <Button
      type="button"
      variant="outline"
      size="sm"
      onClick={openDialog}
      disabled={sync.loading}
      className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid="prompt-sync-open"
    >
      <IconRefresh className="h-4 w-4 mr-2" />
      {sync.config ? t("settings:promptSyncConfigure") : t("settings:promptSyncSetUp")}
    </Button>
  );

  return (
    <SettingsGroup
      title={t("settings:promptSyncTitle")}
      description={t("settings:promptSyncDescription")}
      action={sync.config ? undefined : configureButton}
      data-testid="prompt-sync-section"
    >
      {sync.config && (
        <WorkflowSyncStatusCard
          config={sync.config}
          syncing={sync.syncing}
          onSyncNow={sync.handleSyncNow}
          testIdPrefix={TEST_ID_PREFIX}
          actions={configureButton}
        />
      )}
      <PromptSyncDialog open={open} onOpenChange={setOpen} sync={sync} />
    </SettingsGroup>
  );
}
