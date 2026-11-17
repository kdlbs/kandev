"use client";

import { useTranslation } from "react-i18next";
import { IconLoader2, IconPlus, IconRefresh, IconTerminal2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { settingsActionClassName } from "./settings-control";
import { AgentOptionsDialog } from "@/app/settings/agents/agent-options-dialog";

const actionClassName = settingsActionClassName("cursor-pointer");

export function InstalledAgentsHeader({
  rescanning,
  onOpenShell,
  onOpenTuiDialog,
  onRescan,
}: {
  rescanning: boolean;
  onOpenShell: () => void;
  onOpenTuiDialog?: () => void;
  onRescan: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex w-full flex-wrap gap-2 md:w-auto" data-testid="installed-agents-actions">
      <AgentOptionsDialog />
      <Button
        variant="outline"
        onClick={onOpenShell}
        className={actionClassName}
        data-testid="open-host-shell"
      >
        <IconTerminal2 className="h-4 w-4 mr-2" />
        {t("agents:terminal")}
      </Button>
      <Button
        variant="outline"
        onClick={onRescan}
        disabled={rescanning}
        className={actionClassName}
        data-testid="rescan-agents-button"
      >
        {rescanning ? (
          <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
        ) : (
          <IconRefresh className="h-4 w-4 mr-2" />
        )}
        {t("agents:rescan")}
      </Button>
      {onOpenTuiDialog && (
        <Button
          variant="outline"
          onClick={onOpenTuiDialog}
          className={actionClassName}
          data-testid="new-agent-button"
        >
          <IconPlus className="h-4 w-4 mr-2" />
          {t("agents:addTuiAgent")}
        </Button>
      )}
    </div>
  );
}
