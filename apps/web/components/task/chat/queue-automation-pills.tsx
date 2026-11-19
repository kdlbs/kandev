import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import type { QueuePanelHeaderProps } from "./queued-ghost-panel-header";
type QueueAutomationPillsProps = Pick<
  QueuePanelHeaderProps,
  | "autoRun"
  | "autoMerge"
  | "autoMergeAvailable"
  | "isLoading"
  | "cancellationPending"
  | "onAutoRunChange"
  | "onAutoMergeChange"
>;

export function QueueAutomationPills({
  autoRun,
  autoMerge,
  autoMergeAvailable,
  isLoading,
  cancellationPending,
  onAutoRunChange,
  onAutoMergeChange,
}: QueueAutomationPillsProps) {
  const { t } = useTranslation();
  const autoRunId = useId();
  const autoRunDescriptionId = useId();
  const autoMergeId = useId();
  const controlsDisabled = isLoading || cancellationPending;
  const pillClassName =
    "flex min-h-7 items-center gap-1.5 rounded-full border border-border/70 bg-muted/40 px-2 text-xs font-medium text-foreground [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:px-3";
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <label htmlFor={autoRunId} className={cn(pillClassName, "cursor-pointer")}>
            <span>{t("chat:queueAutoRun")}</span>
            <Switch
              id={autoRunId}
              data-testid="queue-auto-run"
              checked={autoRun}
              disabled={controlsDisabled}
              onCheckedChange={onAutoRunChange}
              aria-label={t("chat:queueAutoRun")}
              aria-describedby={autoRunDescriptionId}
              className="[@media(pointer:coarse)]:after:-inset-y-3.5"
            />
          </label>
        </TooltipTrigger>
        <TooltipContent data-queue-escape-boundary className="max-w-[280px]">
          {t(autoRun ? "chat:queueAutoRunOnHelp" : "chat:queueAutoRunOffHelp")}
        </TooltipContent>
      </Tooltip>
      <span id={autoRunDescriptionId} className="sr-only">
        {t(autoRun ? "chat:queueAutoRunOnHelp" : "chat:queueAutoRunOffHelp")}
      </span>
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={autoMergeAvailable ? -1 : 0} className="inline-flex">
            <label
              htmlFor={autoMergeId}
              className={cn(
                pillClassName,
                autoMergeAvailable ? "cursor-pointer" : "opacity-50 pointer-events-none",
              )}
            >
              <span>{t("chat:queueAutoMerge")}</span>
              <Switch
                id={autoMergeId}
                data-testid="queue-auto-merge"
                checked={autoMerge}
                disabled={controlsDisabled || !autoMergeAvailable}
                onCheckedChange={onAutoMergeChange}
                aria-label={t("chat:queueAutoMerge")}
                className="[@media(pointer:coarse)]:after:-inset-y-3.5"
              />
            </label>
          </span>
        </TooltipTrigger>
        <TooltipContent data-queue-escape-boundary className="max-w-[280px]">
          {t("system:messageQueueAutoMergeSessionNotice")}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}
