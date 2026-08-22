import { IconLayoutList, IconPin, IconPinned, IconTrash, IconX } from "@tabler/icons-react";
import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui";
import { Switch } from "@kandev/ui/switch";
import { cn } from "@/lib/utils";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export type QueuePanelHeaderProps = {
  count: number;
  max: number;
  isFull: boolean;
  autoRun: boolean;
  isLoading: boolean;
  cancellationPending: boolean;
  pinned: boolean;
  onClear: () => void;
  onAutoRunChange: (enabled: boolean) => void;
  onTogglePin: () => void;
  onClose: () => void;
};

export function QueuePanelHeader({
  count,
  max,
  isFull,
  autoRun,
  isLoading,
  cancellationPending,
  pinned,
  onClear,
  onAutoRunChange,
  onTogglePin,
  onClose,
}: QueuePanelHeaderProps) {
  const { t } = useTranslation();
  const autoRunId = useId();
  const autoRunHelpId = `${autoRunId}-help`;
  // The pin is a desktop-only convenience: on phone viewports the queue panel
  // keeps its existing controls and the pin is not rendered.
  const { isMobile } = useResponsiveBreakpoint();
  const capacityText =
    max > 0
      ? t(isFull ? "chat:queueCapacityFull" : "chat:queueCapacity", { count, max })
      : t("chat:queueCount", { count });
  return (
    <div className="flex shrink-0 flex-col py-1">
      <div className="flex min-w-0 items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <IconLayoutList className="h-3.5 w-3.5 shrink-0" />
          <span className="uppercase tracking-wide">{t("chat:queued")}</span>
          <span className={cn("truncate", isFull && "text-amber-600 dark:text-amber-400")}>
            {capacityText}
          </span>
        </div>
        <div className="flex shrink-0 items-center justify-end gap-1">
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
              {pinned ? (
                <IconPinned className="h-3.5 w-3.5" />
              ) : (
                <IconPin className="h-3.5 w-3.5" />
              )}
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
      <div className="flex min-h-9 min-w-0 items-center justify-between gap-3 py-1 [@media(pointer:coarse)]:min-h-11">
        <label htmlFor={autoRunId} className="min-w-0 cursor-pointer">
          <span className="block text-xs font-medium text-foreground">
            {t("chat:queueAutoRun")}
          </span>
          <span id={autoRunHelpId} className="block truncate text-[11px] text-muted-foreground">
            {t(autoRun ? "chat:queueAutoRunOnHelp" : "chat:queueAutoRunOffHelp")}
          </span>
        </label>
        <Switch
          id={autoRunId}
          data-testid="queue-auto-run"
          checked={autoRun}
          disabled={isLoading || cancellationPending}
          onCheckedChange={onAutoRunChange}
          aria-describedby={autoRunHelpId}
          aria-label={t("chat:queueAutoRun")}
          className="[@media(pointer:coarse)]:after:-inset-y-3.5"
        />
      </div>
    </div>
  );
}
