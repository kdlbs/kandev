"use client";

import { useCallback, useState } from "react";
import {
  IconAlertTriangle,
  IconCircleCheck,
  IconPlayerPlay,
  IconPlus,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";

export type SessionStoppedBannerMode = "recoverable" | "completed";

export type SessionStoppedBannerProps = {
  mode: SessionStoppedBannerMode;
  showDialog: boolean;
  onShowDialog: (open: boolean) => void;
  taskId: string | null;
  sessionId: string | null;
  workspaceId?: string | null;
  message?: string;
  detail?: string;
  resumeLabel?: string;
  resumingLabel?: string;
};

async function requestSessionRecover(
  taskId: string,
  sessionId: string,
  action: "resume" | "fresh_start",
): Promise<boolean> {
  const client = getWebSocketClient();
  if (!client) return false;
  try {
    await client.request(
      "session.recover",
      { task_id: taskId, session_id: sessionId, action },
      30000,
    );
    return true;
  } catch {
    return false;
  }
}

function RecoverableSessionActions({
  onShowDialog,
  taskId,
  sessionId,
  resumeLabel,
  resumingLabel,
}: Pick<
  SessionStoppedBannerProps,
  "onShowDialog" | "taskId" | "sessionId" | "resumeLabel" | "resumingLabel"
>) {
  const { t } = useTranslation();
  const resumeText = resumeLabel ?? t("task:resume");
  const resumingText = resumingLabel ?? t("task:resuming");
  const [isResuming, setIsResuming] = useState(false);
  const [isStartingFresh, setIsStartingFresh] = useState(false);

  const profileExists = useAppStore((s) => {
    if (!sessionId) return false;
    const agentProfileId = s.taskSessions.items[sessionId]?.agent_profile_id;
    return (
      !!agentProfileId && s.agentProfiles.items.some((p: { id: string }) => p.id === agentProfileId)
    );
  });

  const handleRecover = useCallback(
    async (action: "resume" | "fresh_start") => {
      if (!sessionId || !taskId) return;
      const setBusy = action === "resume" ? setIsResuming : setIsStartingFresh;
      setBusy(true);
      const ok = await requestSessionRecover(taskId, sessionId, action);
      if (!ok) setBusy(false);
    },
    [sessionId, taskId],
  );

  const handleResume = useCallback(() => handleRecover("resume"), [handleRecover]);
  const handleFreshStart = useCallback(() => {
    if (!profileExists) {
      onShowDialog(true);
      return;
    }
    void handleRecover("fresh_start");
  }, [profileExists, onShowDialog, handleRecover]);

  return (
    <div className="flex w-full flex-col gap-2 sm:w-auto sm:flex-row">
      {sessionId && taskId && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              className="inline-flex w-full sm:w-auto"
              data-testid="failed-session-resume-wrapper"
            >
              <Button
                variant="default"
                size="sm"
                data-testid="recovery-resume-button"
                className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:w-auto"
                onClick={handleResume}
                disabled={isResuming || !profileExists}
              >
                <IconPlayerPlay className="h-3.5 w-3.5" />
                {isResuming ? resumingText : resumeText}
              </Button>
            </span>
          </TooltipTrigger>
          {!profileExists && (
            <TooltipContent>{t("task:agentProfileNoLongerExists")}</TooltipContent>
          )}
        </Tooltip>
      )}
      <Button
        variant="outline"
        size="sm"
        className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:w-auto"
        onClick={handleFreshStart}
        disabled={isStartingFresh}
        data-testid="recovery-fresh-button"
      >
        <IconRefresh className="h-3.5 w-3.5" />
        {isStartingFresh ? t("task:starting") : t("task:startFreshSession")}
      </Button>
    </div>
  );
}

export function SessionStoppedBanner({
  mode,
  showDialog,
  onShowDialog,
  taskId,
  sessionId,
  workspaceId,
  message,
  detail,
  resumeLabel,
  resumingLabel,
}: SessionStoppedBannerProps) {
  const { t } = useTranslation();
  const isCompleted = mode === "completed";
  const bannerMessage = isCompleted
    ? t("task:sessionCompleted")
    : (message ?? t("task:agentHasStopped"));

  return (
    <>
      <div
        data-testid={isCompleted ? "completed-session-banner" : "failed-session-banner"}
        data-session-stopped-mode={mode}
        className="overflow-hidden rounded border border-border"
      >
        <div className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 flex-1 items-start gap-2 text-sm text-muted-foreground">
            {isCompleted ? (
              <IconCircleCheck className="mt-0.5 h-4 w-4 shrink-0 text-green-500" />
            ) : (
              <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-orange-500" />
            )}
            <span className="min-w-0 break-words">{bannerMessage}</span>
            {detail && (
              <span className="min-w-0 break-words text-xs text-muted-foreground">({detail})</span>
            )}
          </div>

          {isCompleted ? (
            <Button
              variant="default"
              size="sm"
              data-testid="completed-session-new-agent-button"
              className="min-h-11 w-full shrink-0 gap-1.5 cursor-pointer sm:w-auto"
              onClick={() => {
                if (taskId) onShowDialog(true);
              }}
              disabled={!taskId}
            >
              <IconPlus className="h-3.5 w-3.5" />
              {t("task:newAgent")}
            </Button>
          ) : (
            <RecoverableSessionActions
              onShowDialog={onShowDialog}
              taskId={taskId}
              sessionId={sessionId}
              resumeLabel={resumeLabel}
              resumingLabel={resumingLabel}
            />
          )}
        </div>
      </div>
      {taskId && (
        <NewSessionDialog
          open={showDialog}
          onOpenChange={onShowDialog}
          taskId={taskId}
          workspaceId={workspaceId}
        />
      )}
    </>
  );
}
