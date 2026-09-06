"use client";

import {
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  type SyntheticEvent,
} from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { getExecutorStatusIcon } from "@/lib/executor-icons";
import { t } from "@/lib/i18n";
import { useRemoteExecutorStatus } from "@/hooks/domains/session/use-remote-executor-status";
import type { RemoteExecutorStatusData } from "@/hooks/domains/session/remote-executor-status-resource";
import { cn } from "@/lib/utils";
import { RemoteExecutorTaskStatusSummary } from "./remote-executor-task-status-summary";
import { useTaskIconTooltipState } from "./use-task-icon-tooltip-state";

type RemoteCloudTooltipProps = {
  taskId: string;
  sessionId?: string | null;
  executorId?: string | null;
  executorType?: string | null;
  fallbackName?: string | null;
  iconClassName?: string;
  /** When provided, uses this data directly instead of fetching via WS on hover. */
  status?: RemoteExecutorStatusData | null;
};

const CONNECTED_THRESHOLD_MS = 2 * 60 * 1000;

function getCloudState(status: RemoteExecutorStatusData | null): "connected" | "error" | "stale" {
  if (status?.remote_status_error) return "error";
  if (!status?.remote_checked_at) return "stale";
  const elapsed = Date.now() - new Date(status.remote_checked_at).getTime();
  if (elapsed < CONNECTED_THRESHOLD_MS) return "connected";
  return "stale";
}

const CLOUD_STATE_CLASSES: Record<ReturnType<typeof getCloudState>, string> = {
  error: "text-destructive",
  connected: "text-emerald-500",
  stale: "text-muted-foreground",
};

type StatusTriggerProps = {
  touch: boolean;
  expanded: boolean;
  label: string;
  icon: ReturnType<typeof getExecutorStatusIcon>;
  iconClassName: string;
  cloudState: ReturnType<typeof getCloudState>;
  onPointerEnter?: ReturnType<typeof useTaskIconTooltipState>["onPointerEnter"];
  onPointerLeave?: ReturnType<typeof useTaskIconTooltipState>["onPointerLeave"];
  onFocus?: ReturnType<typeof useTaskIconTooltipState>["onFocus"];
  onBlur?: ReturnType<typeof useTaskIconTooltipState>["onBlur"];
  onTouchClick?: (event: MouseEvent<HTMLSpanElement>) => void;
  onTouchKeyDown?: (event: KeyboardEvent<HTMLSpanElement>) => void;
};

function StatusTrigger({
  touch,
  expanded,
  label,
  icon,
  iconClassName,
  cloudState,
  onPointerEnter,
  onPointerLeave,
  onFocus,
  onBlur,
  onTouchClick,
  onTouchKeyDown,
}: StatusTriggerProps) {
  const Icon = icon.Icon;
  return (
    <span
      data-testid="remote-executor-status-trigger"
      role={touch ? "button" : "img"}
      tabIndex={0}
      aria-label={label}
      aria-haspopup={touch ? "dialog" : undefined}
      aria-expanded={touch ? expanded : undefined}
      onPointerEnter={onPointerEnter}
      onPointerLeave={onPointerLeave}
      onFocus={onFocus}
      onBlur={onBlur}
      onClick={onTouchClick}
      onKeyDown={onTouchKeyDown}
      className={cn(
        "relative inline-flex shrink-0 items-center justify-center",
        CLOUD_STATE_CLASSES[cloudState],
        touch &&
          "cursor-pointer transition-transform duration-150 ease-out active:scale-[0.96] after:absolute after:left-1/2 after:top-1/2 after:size-11 after:-translate-x-1/2 after:-translate-y-1/2 after:content-['']",
      )}
    >
      <Icon data-testid={icon.testId} className={iconClassName} />
    </span>
  );
}

type StatusDisclosureProps = {
  label: string;
  remoteName: string;
  summary: ReactNode;
  icon: ReturnType<typeof getExecutorStatusIcon>;
  iconClassName: string;
  cloudState: ReturnType<typeof getCloudState>;
  onRefresh: () => void;
};

function stopTaskRowInteraction(event: SyntheticEvent) {
  event.stopPropagation();
}

function TouchStatusDisclosure({
  label,
  remoteName,
  summary,
  icon,
  iconClassName,
  cloudState,
  onRefresh,
}: StatusDisclosureProps) {
  const [open, setOpen] = useState(false);

  function show(event: MouseEvent<HTMLSpanElement> | KeyboardEvent<HTMLSpanElement>) {
    event.preventDefault();
    event.stopPropagation();
    setOpen(true);
    onRefresh();
  }

  function handleKeyDown(event: KeyboardEvent<HTMLSpanElement>) {
    if ((event.key === "Enter" || event.key === " ") && !event.repeat) show(event);
  }

  return (
    <span
      className="contents"
      onClick={stopTaskRowInteraction}
      onKeyDown={stopTaskRowInteraction}
      onMouseDown={stopTaskRowInteraction}
      onPointerDown={stopTaskRowInteraction}
      onTouchStart={stopTaskRowInteraction}
    >
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerTrigger asChild>
          <StatusTrigger
            touch
            expanded={open}
            label={label}
            icon={icon}
            iconClassName={iconClassName}
            cloudState={cloudState}
            onTouchClick={show}
            onTouchKeyDown={handleKeyDown}
          />
        </DrawerTrigger>
        <DrawerContent
          data-testid="remote-executor-status-drawer"
          className="max-h-[min(32rem,calc(100dvh-1rem))] overflow-hidden pb-[env(safe-area-inset-bottom,0px)]"
        >
          <DrawerHeader className="flex shrink-0 flex-row items-center justify-between border-b px-4 py-2 text-left">
            <div className="min-w-0">
              <DrawerTitle className="truncate text-sm">{remoteName}</DrawerTitle>
              <DrawerDescription className="sr-only">{label}</DrawerDescription>
            </div>
            <DrawerClose asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-11 w-11 shrink-0 cursor-pointer transition-transform duration-150 ease-out active:scale-[0.96]"
                aria-label={t("common:close")}
              >
                <IconX aria-hidden="true" className="size-4" />
              </Button>
            </DrawerClose>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto overscroll-contain p-4" data-vaul-no-drag>
            {summary}
          </div>
        </DrawerContent>
      </Drawer>
    </span>
  );
}

function FinePointerStatusTooltip({
  label,
  summary,
  icon,
  iconClassName,
  cloudState,
  onRefresh,
}: StatusDisclosureProps) {
  const tooltip = useTaskIconTooltipState(onRefresh);
  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <StatusTrigger
          touch={false}
          expanded={tooltip.open}
          label={label}
          icon={icon}
          iconClassName={iconClassName}
          cloudState={cloudState}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
        />
      </TooltipTrigger>
      <TooltipContent
        side="top"
        sideOffset={6}
        onEscapeKeyDown={tooltip.onEscapeKeyDown}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        {summary}
      </TooltipContent>
    </Tooltip>
  );
}

export function RemoteCloudTooltip({
  taskId,
  sessionId,
  executorId,
  executorType,
  fallbackName,
  iconClassName = "h-3.5 w-3.5",
  status: externalStatus,
}: RemoteCloudTooltipProps) {
  const hasExternalStatus = externalStatus !== undefined;
  const usesTouchDrawer = useTouchDrawer();
  const live = useRemoteExecutorStatus(
    {
      executorId,
      executorType,
      taskId,
      sessionId: sessionId ?? "",
    },
    !hasExternalStatus,
  );
  const status = hasExternalStatus ? (externalStatus ?? null) : live.status;
  const remoteName = status?.remote_name ?? fallbackName ?? t("task:remoteExecutor");
  const cloudState = getCloudState(status);
  const loading = live.loading;
  const icon = getExecutorStatusIcon(executorType, cloudState === "error");
  const label = t("task:remoteExecutorStatus", { name: remoteName });
  const summary = (
    <RemoteExecutorTaskStatusSummary
      executorType={executorType}
      remoteName={remoteName}
      status={status}
      loading={loading}
    />
  );

  if (usesTouchDrawer) {
    return (
      <TouchStatusDisclosure
        label={label}
        remoteName={remoteName}
        summary={summary}
        icon={icon}
        iconClassName={iconClassName}
        cloudState={cloudState}
        onRefresh={() => void live.refresh()}
      />
    );
  }

  return (
    <FinePointerStatusTooltip
      label={label}
      remoteName={remoteName}
      summary={summary}
      icon={icon}
      iconClassName={iconClassName}
      cloudState={cloudState}
      onRefresh={() => void live.refresh()}
    />
  );
}
