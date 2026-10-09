"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconAdjustmentsHorizontal,
  IconAlertTriangle,
  IconRefresh,
  IconPlayerPause,
  IconPlayerPlay,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle, DrawerTrigger } from "@kandev/ui/drawer";
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
      className="min-h-11 cursor-pointer gap-1.5 sm:min-h-0"
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
export type WorkspacePauseViewProps = {
  activeWorkspaceId: string | null;
  record: WorkspacePauseRecord | null;
  status: ReturnType<typeof useWorkspacePause>["status"];
  refresh: () => Promise<void>;
  pause: UseWorkspacePauseResult["pause"];
  retryPause: UseWorkspacePauseResult["retryPause"];
  resume: UseWorkspacePauseResult["resume"];
  sweep: UseWorkspacePauseResult["sweep"];
  isMutating: boolean;
};

function PausedBanner({
  record,
  stale,
  sweep,
  onRetryPause,
  isMutating,
}: {
  record: WorkspacePauseRecord;
  stale: boolean;
  sweep: UseWorkspacePauseResult["sweep"];
  onRetryPause: UseWorkspacePauseResult["retryPause"];
  isMutating: boolean;
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
        {sweep && sweep.failures > 0 && (
          <div
            className="mt-2 flex flex-wrap items-center gap-2 text-destructive"
            data-testid="office-pause-partial-failure"
          >
            <span>{t("office:pauseWorkspacePartialFailure", { count: sweep.failures })}</span>
            <Button
              size="sm"
              variant="ghost"
              className="min-h-11 cursor-pointer px-2 sm:min-h-0"
              disabled={isMutating}
              data-testid="office-pause-retry-button"
              onClick={() => void onRetryPause()}
            >
              {t("office:retryPauseSweep")}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

// AC-OFFICE-KILL-SWITCH-006.4's "Frontend state" unknown-with-no-record case:
// the absence of a banner must never be read as "running", so an explicit
// affordance is shown instead. The pause control stays reachable here too.
function PauseStateUnavailableBar() {
  const { t } = useTranslation();
  return (
    <div
      className="flex flex-wrap items-center gap-3 rounded-lg border border-amber-300 bg-amber-50 px-3 py-2 text-sm dark:bg-amber-950/30"
      data-testid="office-workspace-pause-unavailable"
    >
      <IconAlertTriangle className="h-4 w-4 shrink-0 text-amber-700 dark:text-amber-300" />
      <span className="min-w-0 flex-1">{t("office:pauseStateUnavailable")}</span>
    </div>
  );
}

function WorkspacePauseActions({ view }: { view: WorkspacePauseViewProps }) {
  const { t } = useTranslation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  if (!view.activeWorkspaceId) return null;
  const renderActions = (onOpen: () => void) => {
    const controls = (
      <div className="flex flex-wrap items-center gap-2">
        <RefreshPauseStateButton onRefresh={view.refresh} testId="office-pause-refresh-topbar" />
        <Button
          size="sm"
          variant={view.record ? "default" : "outline"}
          className="min-h-11 cursor-pointer gap-1.5 sm:min-h-0"
          disabled={view.isMutating}
          data-testid={
            view.record ? "office-resume-workspace-button" : "office-pause-workspace-button"
          }
          onClick={() => {
            setDrawerOpen(false);
            onOpen();
          }}
        >
          {view.record ? (
            <IconPlayerPlay className="h-3.5 w-3.5" />
          ) : (
            <IconPlayerPause className="h-3.5 w-3.5" />
          )}
          {t(view.record ? "office:resumeWorkspace" : "office:pauseWorkspace")}
        </Button>
      </div>
    );
    return (
      <>
        <div
          className="hidden items-center gap-2 md:flex"
          data-testid="office-workspace-topbar-actions"
        >
          {controls}
        </div>
        <div className="md:hidden">
          <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
            <DrawerTrigger asChild>
              <Button
                variant="ghost"
                className="min-h-11 min-w-11 cursor-pointer px-2"
                aria-label={t("office:workspaceActions")}
                data-testid="office-workspace-actions-trigger"
              >
                <IconAdjustmentsHorizontal className="h-4 w-4" />
              </Button>
            </DrawerTrigger>
            <DrawerContent data-testid="office-workspace-actions-drawer">
              <DrawerHeader>
                <DrawerTitle>{t("office:workspaceActions")}</DrawerTitle>
              </DrawerHeader>
              <div className="flex flex-col gap-2 px-4 pb-6 [&_button]:min-h-11">{controls}</div>
            </DrawerContent>
          </Drawer>
        </div>
      </>
    );
  };
  return view.record ? (
    <ResumeWorkspaceButton onResume={view.resume} renderTrigger={renderActions} />
  ) : (
    <PauseWorkspaceButton onPause={view.pause} renderTrigger={renderActions} />
  );
}

export function WorkspacePauseTopbarActions({ view }: { view: WorkspacePauseViewProps }) {
  return <WorkspacePauseActions view={view} />;
}

export function WorkspacePauseState({ view }: { view: WorkspacePauseViewProps }) {
  if (!view.activeWorkspaceId) return null;
  if (view.record) {
    return (
      <PausedBanner
        record={view.record}
        stale={view.status === "unknown"}
        sweep={view.sweep}
        onRetryPause={view.retryPause}
        isMutating={view.isMutating}
      />
    );
  }
  if (view.status === "unknown") return <PauseStateUnavailableBar />;
  return null;
}

/**
 * Mounted once by `OfficeShell` so it survives every `/office/**` navigation
 * (AC-OFFICE-KILL-SWITCH-006.4, -006.12). Renders nothing until a workspace
 * is selected — there is no workspace to pause on, for example, the setup
 * page.
 */
export function WorkspacePauseBanner() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const { record, status, refresh, pause, retryPause, resume, sweep, isMutating } =
    useWorkspacePause(activeWorkspaceId);
  return (
    <WorkspacePauseState
      view={{
        activeWorkspaceId,
        record,
        status,
        refresh,
        pause,
        retryPause,
        resume,
        sweep,
        isMutating,
      }}
    />
  );
}
