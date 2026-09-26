import { IconLayoutList, IconPin, IconPinned, IconTrash, IconX } from "@tabler/icons-react";
import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export type QueuePanelHeaderProps = {
  count: number;
  max: number;
  isFull: boolean;
  autoRun: boolean;
  autoMerge: boolean;
  autoMergeAvailable: boolean;
  isLoading: boolean;
  cancellationPending: boolean;
  pinned: boolean;
  onClear: () => void;
  onAutoRunChange: (enabled: boolean) => void;
  onAutoMergeChange: (enabled: boolean) => void;
  onTogglePin: () => void;
  onClose: () => void;
};

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

function QueueAutomationPills({
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
        <TooltipContent className="max-w-[280px]">
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
        <TooltipContent className="max-w-[280px]">
          {t("system:messageQueueAutoMergeNotice")}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

export function QueuePanelHeader({
  count,
  max,
  isFull,
  autoRun,
  autoMerge,
  autoMergeAvailable,
  isLoading,
  cancellationPending,
  pinned,
  onClear,
  onAutoRunChange,
  onAutoMergeChange,
  onTogglePin,
  onClose,
}: QueuePanelHeaderProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const capacityText =
    max > 0
      ? t(isFull ? "chat:queueCapacityFull" : "chat:queueCapacity", { count, max })
      : t("chat:queueCount", { count });

  return (
    <div className="flex min-w-0 shrink-0 flex-wrap items-center gap-2 py-1">
      <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
        <IconLayoutList className="h-3.5 w-3.5 shrink-0" />
        <span className="uppercase tracking-wide">{t("chat:queued")}</span>
        <span className={cn("truncate", isFull && "text-amber-600 dark:text-amber-400")}>
          {capacityText}
        </span>
      </div>
      <QueueAutomationPills
        autoRun={autoRun}
        autoMerge={autoMerge}
        autoMergeAvailable={autoMergeAvailable}
        isLoading={isLoading}
        cancellationPending={cancellationPending}
        onAutoRunChange={onAutoRunChange}
        onAutoMergeChange={onAutoMergeChange}
      />
      <div className="ml-auto flex shrink-0 items-center justify-end gap-1">
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:px-3"
          onClick={onClear}
          title={t("chat:clearAllQueuedMessages")}
          data-testid="queue-clear-all"
        >
          <IconTrash className="mr-1 h-3 w-3" />
          {t("chat:clearAll")}
        </Button>
        {!isMobile && (
          <button
            type="button"
            onClick={onTogglePin}
            aria-pressed={pinned}
            aria-label={t(pinned ? "chat:unpinQueuedMessages" : "chat:pinQueuedMessages")}
            title={t(pinned ? "chat:unpinQueuedMessages" : "chat:pinQueuedMessages")}
            data-testid="queue-pin"
            className="inline-flex h-6 w-6 items-center justify-center text-muted-foreground hover:text-foreground cursor-pointer rounded p-1 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
          >
            {pinned ? <IconPinned className="h-3.5 w-3.5" /> : <IconPin className="h-3.5 w-3.5" />}
          </button>
        )}
        <button
          type="button"
          onClick={onClose}
          aria-label={t("chat:collapseQueuedMessages")}
          data-testid="queue-close"
          className="inline-flex h-6 w-6 items-center justify-center text-muted-foreground hover:text-foreground cursor-pointer rounded p-1 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
        >
          <IconX className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}
