"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";
import { useExecutorFailure } from "@/hooks/use-executor-failure";
import { executorFailureTitleKey } from "@/lib/executor-failure";
import { useTaskLaunchErrorContext } from "../task-launch-error-context";
import { executorFailureForOwner } from "@/lib/executor-failure";
import { ExecutorFailureDetails } from "../executor-failure-details";

export function SessionExecutorFailureCard({
  taskId,
  episode,
}: {
  taskId: string;
  episode: ExecutorFailureEpisode;
}) {
  const recovery = useExecutorFailure(taskId, episode);
  return <SessionExecutorFailureCardView taskId={taskId} episode={episode} recovery={recovery} />;
}

export function SessionExecutorFailureCardView({
  taskId,
  episode,
  recovery,
}: {
  taskId: string;
  episode: ExecutorFailureEpisode;
  recovery: ReturnType<typeof useExecutorFailure>;
}) {
  const [expanded, setExpanded] = useState(false);
  const { t } = useTranslation();
  const failure = recovery.episode;
  if (failure?.state !== "active") return null;
  const worker = failure.observation.reason === "WorkerUnavailable";
  return (
    <section
      id={`session-recovery-${episode.session_id ?? taskId}`}
      tabIndex={-1}
      className="max-h-[50dvh] min-h-0 min-w-0 overflow-y-auto overscroll-contain rounded-md border border-amber-500/35 bg-card"
      data-testid="session-executor-failure-card"
    >
      <div className="flex min-w-0 gap-3 border-b border-amber-500/20 bg-amber-500/5 p-3 dark:bg-amber-500/10 md:p-4">
        <IconAlertTriangle
          className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-medium">{t(executorFailureTitleKey(failure))}</h3>
          {!worker && failure.observation.message && (
            <p className="mt-1 text-sm wrap-anywhere text-muted-foreground">
              {failure.observation.message}
            </p>
          )}
          <p className="mt-1 text-sm text-muted-foreground">
            {t(worker ? "task:executorFailureWorkerGuidance" : "task:executorFailureGuidance")}
          </p>
        </div>
      </div>
      <div className="min-w-0 px-3 pb-3 pt-3 md:px-4 md:pb-4">
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className={controlSizingClassName("standard", "cursor-pointer")}
            disabled={recovery.pending}
            onClick={recovery.recheck}
            data-testid="executor-recheck"
          >
            {t(recovery.pending ? "task:executorFailureChecking" : "task:executorFailureRecheck")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className={controlSizingClassName("standard", "cursor-pointer")}
            aria-expanded={expanded}
            aria-controls={`executor-details-${episode.id}`}
            onClick={() => setExpanded(!expanded)}
            data-testid="executor-failure-expand"
          >
            {t(expanded ? "task:hideDetails" : "task:showDetails")}
            <IconChevronDown
              className={`size-3.5 ${expanded ? "rotate-180" : ""}`}
              aria-hidden="true"
            />
          </Button>
        </div>
        {recovery.unverified && (
          <p role="status" className="mt-2 text-sm text-muted-foreground">
            {t("task:executorFailureUnverified")}
          </p>
        )}
        {expanded && (
          <div id={`executor-details-${episode.id}`} className="mt-3 border-t pt-3">
            <ExecutorFailureDetails episode={failure} />
          </div>
        )}
      </div>
    </section>
  );
}

/** Tasks without a session retain a recovery card at the bottom of the workbench. */
export function TaskExecutorFailureFallback() {
  const context = useTaskLaunchErrorContext();
  const episode = executorFailureForOwner(context?.statusSummary?.executor_failure);
  return context && episode ? (
    <div className="shrink-0 p-3">
      <SessionExecutorFailureCard taskId={context.taskId} episode={episode} />
    </div>
  ) : null;
}
