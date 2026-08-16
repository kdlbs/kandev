"use client";

import { useCallback, useState } from "react";
import { IconLoader2, IconSparkles, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import Link from "@/components/routing/app-link";
import { cancelTaskReview, runTaskReview } from "@/lib/api/domains/review-api";
import { isRunActive } from "@/lib/review/findings";
import type { TaskReviewRun } from "@/lib/types/review";
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

export type ReviewRunButtonProps = {
  taskId: string | null | undefined;
  sessionId?: string | null;
  activeRun: TaskReviewRun | null;
  /** Compact presentation for a toolbar with limited width. */
  compact?: boolean;
};

/**
 * Copy for a rejected run. `review_agent_unavailable` is the one case the user
 * can actually fix, so it links straight to the settings page rather than
 * showing a dead-end error.
 */
function RunNotice({ code, message }: { code: string; message: string }) {
  const { t } = useTranslation();
  if (code === "review_agent_unavailable") {
    return (
      <p className="text-xs text-muted-foreground" data-testid="review-agent-unavailable">
        <Trans i18nKey="review:reviewAgentUnavailableNotice">
          No inference-capable agent is configured for review.{" "}
          <Link href="/settings/utility-agents" className="cursor-pointer underline" />
        </Trans>
      </p>
    );
  }
  if (code === "review_no_changes") {
    return (
      <p className="text-xs text-muted-foreground" data-testid="review-no-changes">
        {t("review:noChangesToReviewYet")}
      </p>
    );
  }
  return (
    <p className="text-xs text-destructive" data-testid="review-run-error">
      {message}
    </p>
  );
}

function useRunReview(taskId: string | null | undefined, sessionId?: string | null) {
  const { t } = useTranslation();
  const [notice, setNotice] = useState<{ code: string; message: string } | null>(null);
  const [starting, setStarting] = useState(false);

  const start = useCallback(async () => {
    if (!taskId) return;
    setNotice(null);
    setStarting(true);
    try {
      await runTaskReview({ taskId, sessionId: sessionId ?? undefined });
    } catch (error) {
      const code = (error as { code?: string })?.code ?? "";
      const message = error instanceof Error ? error.message : t("review:couldNotStartReview");
      setNotice({ code, message });
    } finally {
      setStarting(false);
    }
  }, [taskId, sessionId]);

  return { notice, starting, start, clearNotice: () => setNotice(null) };
}

/**
 * Starts a native code-review pass over the task's current changes and reflects
 * the run's state. Findings render as inline comments in the diff; this control
 * only owns the trigger and the run status.
 */
function RunTrigger({
  busy,
  disabled,
  compact,
  onStart,
}: {
  busy: boolean;
  disabled: boolean;
  compact: boolean;
  onStart: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="cursor-pointer gap-1 px-2"
          onClick={onStart}
          disabled={disabled}
          aria-label={t("review:reviewTheseChangesWithAnAgent")}
          data-testid="review-run-changes"
        >
          {busy ? (
            <IconLoader2 className="h-4 w-4 animate-spin" />
          ) : (
            <IconSparkles className="h-4 w-4" />
          )}
          {!compact && <span className="text-xs">{t("review:reviewChanges")}</span>}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {busy ? t("review:reviewInProgress") : t("review:reviewTheseChangesWithAnAgent")}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Picks which notice to show: a rejection from the request the user just made
 * takes precedence over the last run's stored failure, because it is the more
 * recent answer to what they asked for.
 */
function resolveNotice(
  notice: { code: string; message: string } | null,
  activeRun: TaskReviewRun | null,
  t: TFunction,
): { code: string; message: string } | null {
  if (notice) return notice;
  if (activeRun?.status !== "failed") return null;
  return {
    code: activeRun.error_code,
    message: activeRun.error_message || t("review:theReviewFailed"),
  };
}

export function ReviewRunButton({
  taskId,
  sessionId,
  activeRun,
  compact = false,
}: ReviewRunButtonProps) {
  const { t } = useTranslation();
  const { notice, starting, start, clearNotice } = useRunReview(taskId, sessionId);
  const running = isRunActive(activeRun);
  const shown = resolveNotice(notice, activeRun, t);

  const handleCancel = useCallback(async () => {
    if (!activeRun) return;
    clearNotice();
    await cancelTaskReview(activeRun.id).catch(() => {
      // The run is already terminal or unreachable; its row carries the state.
    });
  }, [activeRun, clearNotice]);

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <RunTrigger
        busy={running || starting}
        disabled={!taskId || running || starting}
        compact={compact}
        onStart={start}
      />

      {running && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 cursor-pointer gap-1 px-1.5 text-xs text-muted-foreground"
          onClick={handleCancel}
          data-testid="review-cancel-run"
        >
          <IconX className="h-3.5 w-3.5" />
          {t("common:cancel")}
        </Button>
      )}

      {shown && <RunNotice code={shown.code} message={shown.message} />}
    </div>
  );
}
