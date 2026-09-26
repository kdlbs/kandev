"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { IconInfoCircle } from "@tabler/icons-react";
import type { Automation } from "@/lib/types/automation";
import {
  dismissBoardMoveNotice,
  isBoardMoveNoticeDismissed,
} from "@/lib/automation-board-notice-storage";

/**
 * Tells a workspace, once, that its automations stopped putting cards on the
 * kanban.
 *
 * Withdrawing execution modes made every firing land in one hidden place. That
 * is the right shape, but `task` was the stored DEFAULT, so on upgrade a
 * working setup simply stops producing the cards someone built their day
 * around — with nothing on screen to say the work moved rather than broke. The
 * runs are not lost, and this is where that gets said.
 *
 * It reads a per-automation flag the server derives from a retained column,
 * and shows nothing at all for a workspace whose automations never ran in that
 * mode: they lost nothing and have no news to read.
 */
export function AutomationBoardMoveNotice({
  workspaceId,
  automations,
}: {
  workspaceId: string;
  automations: Automation[];
}) {
  const { t } = useTranslation();
  // Which workspace was dismissed in *this* session, rather than a plain
  // boolean: the settings page keeps this component mounted across a workspace
  // switch, and a bare flag would carry one workspace's dismissal onto the
  // next workspace's first visit.
  const [dismissedWorkspace, setDismissedWorkspace] = useState<string | null>(null);
  const alreadyDismissed = useMemo(() => isBoardMoveNoticeDismissed(workspaceId), [workspaceId]);

  const affected = automations.some((automation) => automation.legacy_board_card === true);
  if (!affected || alreadyDismissed || dismissedWorkspace === workspaceId) return null;

  const handleDismiss = () => {
    dismissBoardMoveNotice(workspaceId);
    setDismissedWorkspace(workspaceId);
  };

  return (
    <Alert className="p-3" data-testid="automation-board-move-notice">
      <IconInfoCircle className="mt-0.5 size-4" aria-hidden="true" />
      <AlertTitle className="text-sm">{t("automations:boardMoveNoticeTitle")}</AlertTitle>
      <AlertDescription className="space-y-2 text-left text-sm">
        <p>{t("automations:boardMoveNoticeBody")}</p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={handleDismiss}
          data-testid="automation-board-move-notice-dismiss"
        >
          {t("automations:boardMoveNoticeDismiss")}
        </Button>
      </AlertDescription>
    </Alert>
  );
}
