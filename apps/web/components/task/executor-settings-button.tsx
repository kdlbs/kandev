"use client";

import { useCallback, useState } from "react";
import { IconBox, IconLoader2, IconTrash } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Button } from "@kandev/ui/button";

import { getExecutorStatusIcon } from "@/lib/executor-icons";
import { TaskResetEnvConfirmDialog } from "./task-reset-env-confirm-dialog";
import type { StatusTone } from "./executor-environment-status";
import { usePrepareSummary } from "@/hooks/domains/session/use-prepare-summary";
import { useTaskEnvironment } from "@/hooks/domains/session/use-task-environment";
import { isPreparingPhase } from "@/lib/prepare/summarize";
import { PrepareStatusSection } from "./executor-prepare-status";
import { EnvironmentInfo } from "./executor-environment-info";

type ExecutorSettingsButtonProps = {
  taskId?: string | null;
  sessionId?: string | null;
  disabled?: boolean;
};

export function ExecutorSettingsButton({
  taskId,
  sessionId,
  disabled,
}: ExecutorSettingsButtonProps) {
  const [open, setOpen] = useState(false);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const prepare = usePrepareSummary(sessionId ?? null);
  const isPreparing = isPreparingPhase(prepare.phase);
  // Promote the foreground polling cadence while preparing so the icon flips
  // to "ready" without the user hovering over the trigger.
  const { env, container, loading, isResetting, reset, status } = useTaskEnvironment(
    taskId,
    open || isPreparing,
  );

  const handleReset = useCallback(
    async (opts: { pushBranch: boolean }) => {
      const ok = await reset(opts);
      if (ok) {
        setResetDialogOpen(false);
        setOpen(false);
      }
    },
    [reset],
  );

  if (!taskId) return null;

  const executorType = env?.executor_type ?? null;
  const ariaLabel = computeAriaLabel(isPreparing, status);

  return (
    <>
      <HoverCard open={open} onOpenChange={setOpen} openDelay={150} closeDelay={250}>
        <HoverCardTrigger asChild>
          {/* Real <button> for free Enter/Space activation, focus rings and
              disabled semantics. variant="ghost" removes the outline so the
              trigger reads as info, not as a clickable action — but we keep
              the click→toggle behaviour the underlying HoverCard offers. */}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={disabled}
            aria-haspopup="dialog"
            aria-label={ariaLabel}
            data-testid="executor-settings-button"
            className="relative h-7 cursor-default gap-1 px-1.5 text-muted-foreground hover:text-foreground"
          >
            <ExecutorButtonIcon executorType={executorType} preparing={isPreparing} />
            <ExecutorStatusDot status={status} loading={loading} />
          </Button>
        </HoverCardTrigger>
        <HoverCardContent
          align="start"
          className="w-[340px] p-0 text-sm"
          data-testid="executor-settings-popover"
        >
          <PrepareStatusSection summary={prepare} />
          <EnvironmentInfo env={env} container={container} loading={loading} />
          <div className="border-t border-border px-2 py-1.5 flex items-center justify-end">
            <Button
              variant="destructive"
              size="sm"
              className="cursor-pointer text-xs"
              disabled={!env || isResetting}
              data-testid="executor-settings-reset"
              onClick={() => setResetDialogOpen(true)}
            >
              <IconTrash className="h-3.5 w-3.5 mr-1" /> Reset environment
            </Button>
          </div>
        </HoverCardContent>
      </HoverCard>

      <TaskResetEnvConfirmDialog
        open={resetDialogOpen}
        onOpenChange={setResetDialogOpen}
        hasWorktreePath={Boolean(env?.worktree_path)}
        isResetting={isResetting}
        onConfirm={handleReset}
      />
    </>
  );
}

function computeAriaLabel(
  preparing: boolean,
  status: { label: string; tone: StatusTone } | null,
): string {
  if (preparing) return "Executor settings, preparing environment";
  if (status) return `Executor settings, environment ${status.label}`;
  return "Executor settings";
}

function ExecutorButtonIcon({
  executorType,
  preparing,
}: {
  executorType: string | null;
  preparing: boolean;
}) {
  if (preparing) {
    return (
      <IconLoader2
        className="h-4 w-4 animate-spin"
        data-testid="executor-settings-button-spinner"
      />
    );
  }
  if (!executorType) {
    return <IconBox className="h-4 w-4" data-testid="executor-status-box-icon" />;
  }
  const { Icon, testId } = getExecutorStatusIcon(executorType, false);
  return <Icon className="h-4 w-4" data-testid={testId} />;
}

const DOT_CLASSES: Record<StatusTone, string> = {
  running: "bg-green-500",
  stopped: "bg-zinc-500",
  warn: "bg-amber-500",
  error: "bg-red-500",
  neutral: "bg-muted-foreground",
};

function ExecutorStatusDot({
  status,
  loading,
}: {
  status: { label: string; tone: StatusTone } | null;
  loading: boolean;
}) {
  const tone = status?.tone ?? "neutral";
  const label = status?.label ?? "not created";
  return (
    <span
      aria-hidden="true"
      title={`Environment ${label}`}
      data-testid="executor-status-indicator"
      className={`absolute right-1 top-1 h-2.5 w-2.5 rounded-full border border-background ${DOT_CLASSES[tone]} ${
        loading && !status ? "animate-pulse" : ""
      }`}
    />
  );
}
