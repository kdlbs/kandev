import {
  IconLayoutList,
  IconPin,
  IconPinned,
  IconPlayerPlay,
  IconSend,
  IconTrash,
  IconX,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui";
import { cn } from "@/lib/utils";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export type QueuePanelHeaderProps = {
  count: number;
  max: number;
  isFull: boolean;
  canDrain: boolean;
  isLoading: boolean;
  cancellationPending: boolean;
  pinned: boolean;
  onClear: () => void;
  onDrain: () => void;
  onSendNow: () => void;
  onTogglePin: () => void;
  onClose: () => void;
};

export function QueuePanelHeader({
  count,
  max,
  isFull,
  canDrain,
  isLoading,
  cancellationPending,
  pinned,
  onClear,
  onDrain,
  onSendNow,
  onTogglePin,
  onClose,
}: QueuePanelHeaderProps) {
  const { t } = useTranslation();
  // The pin is a desktop-only convenience: on phone viewports the queue panel
  // keeps its existing controls and the pin is not rendered.
  const { isMobile } = useResponsiveBreakpoint();
  const capacityText =
    max > 0
      ? t(isFull ? "chat:queueCapacityFull" : "chat:queueCapacity", { count, max })
      : t("chat:queueCount", { count });
  return (
    <div className="flex shrink-0 items-center justify-between gap-3 py-1">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <IconLayoutList className="h-3.5 w-3.5" />
        <span className="uppercase tracking-wide">{t("chat:queued")}</span>
        <span className={cn(isFull && "text-amber-600 dark:text-amber-400")}>{capacityText}</span>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-1">
        {canDrain && (
          <Button
            variant="ghost"
            size="sm"
            className="h-6 px-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:px-3"
            onClick={onDrain}
            disabled={isLoading}
            title={t("chat:runNextQueuedMessage")}
            data-testid="queue-drain-next"
          >
            <IconPlayerPlay className="mr-1 h-3 w-3" />
            {t("chat:runNext")}
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-1.5 text-xs text-muted-foreground hover:text-foreground cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:px-3"
          onClick={onSendNow}
          disabled={isLoading || cancellationPending}
          title={t("chat:sendNowQueuedMessages")}
          data-testid="queue-send-now"
        >
          <IconSend className="mr-1 h-3 w-3" />
          {t("chat:sendNow")}
        </Button>
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
