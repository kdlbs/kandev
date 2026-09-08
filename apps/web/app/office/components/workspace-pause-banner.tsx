"use client";

import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import {
  useWorkspacePause,
  type UseWorkspacePauseResult,
} from "@/hooks/domains/office/use-workspace-pause";
import type { WorkspacePauseRecord } from "@/lib/state/slices/office/types";
import { PauseWorkspaceButton, ResumeWorkspaceButton } from "./workspace-pause-controls";

function RefreshPauseStateButton({
  onRefresh,
  testId,
}: {
  onRefresh: () => Promise<void>;
  testId: string;
}) {
  const { t } = useTranslation();
  return (
    <Button
      size="sm"
      variant="ghost"
      className="cursor-pointer gap-1.5"
      data-testid={testId}
      onClick={() => void onRefresh()}
      title={t("office:refreshPauseState")}
    >
      <IconRefresh className="h-3.5 w-3.5" />
      <span className="sr-only">{t("office:refreshPauseState")}</span>
    </Button>
  );
}

// AC-OFFICE-KILL-SWITCH-006.4: persistent indicator naming actor, reason and
// time, plus a resume control. Rendered whenever a record is present, even
// while `status` is `unknown` (marked stale) — a pause already read stays on
// screen per the design's "Frontend state" GET-failure rule.
function PausedBanner({
  record,
  stale,
  onRefresh,
  onResume,
}: {
  record: WorkspacePauseRecord;
  stale: boolean;
  onRefresh: () => Promise<void>;
  onResume: UseWorkspacePauseResult["resume"];
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm"
      data-testid="office-workspace-paused-banner"
    >
      <IconAlertTriangle className="h-4 w-4 shrink-0 text-destructive" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium">{t("office:workspacePausedTitle")}</span>
          {stale && <Badge variant="outline">{t("office:pauseStateStale")}</Badge>}
        </div>
        <p className="break-words text-muted-foreground">
          {t("office:workspacePausedDetail", {
            actor: record.createdBy,
            reason: record.reason,
            time: new Date(record.createdAt).toLocaleString(),
          })}
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <RefreshPauseStateButton onRefresh={onRefresh} testId="office-pause-refresh-paused" />
        <ResumeWorkspaceButton onResume={onResume} />
      </div>
    </div>
  );
}

// AC-OFFICE-KILL-SWITCH-006.4's "Frontend state" unknown-with-no-record case:
// the absence of a banner must never be read as "running", so an explicit
// affordance is shown instead. The pause control stays reachable here too.
function PauseStateUnavailableBar({
  onRefresh,
  onPause,
}: {
  onRefresh: () => Promise<void>;
  onPause: UseWorkspacePauseResult["pause"];
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-3 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm dark:bg-amber-950/30"
      data-testid="office-workspace-pause-unavailable"
    >
      <IconAlertTriangle className="h-4 w-4 shrink-0 text-amber-700 dark:text-amber-300" />
      <span className="min-w-0 flex-1">{t("office:pauseStateUnavailable")}</span>
      <div className="flex shrink-0 items-center gap-2">
        <RefreshPauseStateButton onRefresh={onRefresh} testId="office-pause-refresh-unavailable" />
        <PauseWorkspaceButton onPause={onPause} />
      </div>
    </div>
  );
}

// Not paused, and pause state is known: only the always-reachable pause
// control plus a refresh, per AC-OFFICE-KILL-SWITCH-006.12/-006.13. No full
// banner — a persistent indicator is only required while paused.
function RunningControlBar({
  onRefresh,
  onPause,
}: {
  onRefresh: () => Promise<void>;
  onPause: UseWorkspacePauseResult["pause"];
}) {
  return (
    <div className="flex items-center justify-end gap-2" data-testid="office-workspace-running-bar">
      <RefreshPauseStateButton onRefresh={onRefresh} testId="office-pause-refresh-running" />
      <PauseWorkspaceButton onPause={onPause} />
    </div>
  );
}

/**
 * Mounted once by `OfficeShell` so it survives every `/office/**` navigation
 * (AC-OFFICE-KILL-SWITCH-006.4, -006.12). Renders nothing until a workspace
 * is selected — there is no workspace to pause on, for example, the setup
 * page.
 */
export function WorkspacePauseBanner() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const { record, status, refresh, pause, resume } = useWorkspacePause(activeWorkspaceId);

  if (!activeWorkspaceId) return null;

  if (record) {
    return (
      <PausedBanner
        record={record}
        stale={status === "unknown"}
        onRefresh={refresh}
        onResume={resume}
      />
    );
  }

  if (status === "unknown") {
    return <PauseStateUnavailableBar onRefresh={refresh} onPause={pause} />;
  }

  return <RunningControlBar onRefresh={refresh} onPause={pause} />;
}
