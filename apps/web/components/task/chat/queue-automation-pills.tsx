import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { cn } from "@/lib/utils";
import type { QueuePanelHeaderProps } from "./queued-ghost-panel-header";
type QueueAutomationPillsProps = Pick<
  QueuePanelHeaderProps,
  | "autoRun"
  | "autoMerge"
  | "autoMergeAvailable"
  | "autoSortMerge"
  | "autoSortMergeAvailable"
  | "isLoading"
  | "cancellationPending"
  | "onAutoRunChange"
  | "onAutoMergeChange"
  | "onAutoSortMergeChange"
>;

export function QueueAutomationPills({
  autoRun,
  autoMerge,
  autoMergeAvailable,
  autoSortMerge,
  autoSortMergeAvailable,
  isLoading,
  cancellationPending,
  onAutoRunChange,
  onAutoMergeChange,
  onAutoSortMergeChange,
}: QueueAutomationPillsProps) {
  const { t } = useTranslation();
  const autoRunId = useId();
  const autoRunDescriptionId = useId();
  const autoMergeId = useId();
  const controlsDisabled = isLoading || cancellationPending;
  const unavailable = !autoMergeAvailable || !autoSortMergeAvailable;
  let sortDescriptionKey = "chat:queueAutoSortMergeHelp";
  if (unavailable) sortDescriptionKey = "chat:queuePolicyUnavailable";
  else if (!autoMerge) sortDescriptionKey = "chat:queueAutoSortMergeRequires";
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
      <AutoSortMergePill
        checked={autoMerge && autoSortMerge}
        disabled={controlsDisabled || unavailable || !autoMerge}
        descriptionKey={sortDescriptionKey}
        onChange={onAutoSortMergeChange}
        pillClassName={pillClassName}
      />
    </div>
  );
}

function AutoSortMergePill({
  checked,
  disabled,
  onChange,
  pillClassName,
  descriptionKey,
}: {
  checked: boolean;
  disabled: boolean;
  onChange: (enabled: boolean) => void;
  pillClassName: string;
  descriptionKey: string;
}) {
  const { t } = useTranslation();
  const id = useId();
  const descriptionId = useId();
  const usesTouchDrawer = useTouchDrawer();
  const pill = (
    <span tabIndex={disabled && !usesTouchDrawer ? 0 : -1} className="inline-flex">
      <label
        htmlFor={id}
        className={cn(
          pillClassName,
          disabled ? "opacity-50 pointer-events-none" : "cursor-pointer",
        )}
      >
        <span>{t("chat:queueAutoSortMerge")}</span>
        <Switch
          id={id}
          data-testid="queue-auto-sort-merge"
          checked={checked}
          disabled={disabled}
          onCheckedChange={onChange}
          aria-label={t("chat:queueAutoSortMerge")}
          aria-describedby={descriptionId}
          className="[@media(pointer:coarse)]:after:-inset-y-3.5"
        />
      </label>
    </span>
  );
  return (
    <>
      {usesTouchDrawer ? (
        <span className="inline-flex items-center">
          {pill}
          <AutoSortMergeTouchHelp description={t(descriptionKey)} />
        </span>
      ) : (
        <Tooltip>
          <TooltipTrigger asChild>{pill}</TooltipTrigger>
          <TooltipContent data-queue-escape-boundary className="max-w-[280px]">
            {t(descriptionKey)}
          </TooltipContent>
        </Tooltip>
      )}
      <span id={descriptionId} className="sr-only">
        {t(descriptionKey)}
      </span>
    </>
  );
}

function AutoSortMergeTouchHelp({ description }: { description: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen} autoFocus>
      <DrawerTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          data-testid="queue-auto-sort-merge-help"
          className={controlSizingClassName(
            "icon",
            "shrink-0 cursor-pointer text-muted-foreground",
          )}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-label={t("chat:queueAutoSortMergeAbout")}
        >
          <IconInfoCircle className="h-4 w-4" />
        </Button>
      </DrawerTrigger>
      <DrawerContent data-queue-escape-boundary>
        <DrawerHeader>
          <DrawerTitle>{t("chat:queueAutoSortMerge")}</DrawerTitle>
          <DrawerDescription>{description}</DrawerDescription>
        </DrawerHeader>
        <div className="p-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          <DrawerClose asChild>
            <Button
              variant="outline"
              data-testid="queue-auto-sort-merge-help-close"
              className="h-11 w-full cursor-pointer"
            >
              {t("common:close")}
            </Button>
          </DrawerClose>
        </div>
      </DrawerContent>
    </Drawer>
  );
}
