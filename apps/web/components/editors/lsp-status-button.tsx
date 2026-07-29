"use client";

import { useEffect, useState, type ComponentProps, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Progress } from "@kandev/ui/progress";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconAlertTriangle,
  IconLoader2,
  IconPlugConnected,
  IconPlugOff,
  IconX,
} from "@tabler/icons-react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { LspStatus } from "@/lib/lsp/lsp-client-manager";
import type { LspProgressSnapshot } from "@/lib/lsp/lsp-progress";
import {
  getLspConnectionLabel,
  getLspLifecycleAction,
  getLspProgressView,
} from "@/lib/lsp/lsp-progress-view";
import { getLspUnavailableSetupHint } from "@/lib/lsp/lsp-json-rpc";

const ICON_CLASS = "h-3.5 w-3.5";

export function LspStatusIcon({
  status,
  progress,
}: {
  status: LspStatus;
  progress: LspProgressSnapshot;
}): ReactNode {
  if (progress.active.length > 0 || progress.initializingSince !== null) {
    return <IconLoader2 className={`${ICON_CLASS} animate-spin text-blue-500`} aria-hidden />;
  }
  switch (status.state) {
    case "connecting":
    case "starting":
    case "stopping":
      return (
        <IconLoader2 className={`${ICON_CLASS} animate-spin text-muted-foreground`} aria-hidden />
      );
    case "installing":
      return <IconLoader2 className={`${ICON_CLASS} animate-spin text-amber-500`} aria-hidden />;
    case "ready":
      return <IconPlugConnected className={`${ICON_CLASS} text-emerald-500`} aria-hidden />;
    case "error":
      return <IconAlertTriangle className={`${ICON_CLASS} text-yellow-500`} aria-hidden />;
    case "disabled":
    case "unavailable":
      return <IconPlugOff className={`${ICON_CLASS} text-muted-foreground`} aria-hidden />;
  }
}

export function useLspLiveNow(enabled: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!enabled) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [enabled]);
  return Math.max(now, Date.now());
}

function ConnectionDetails({
  status,
  lspLanguage,
  progress,
}: {
  status: LspStatus;
  lspLanguage: string;
  progress: LspProgressSnapshot;
}) {
  const reason = "reason" in status ? status.reason : null;
  const setupHint = getLspUnavailableSetupHint(status, lspLanguage);
  return (
    <section aria-labelledby="lsp-connection-heading" className="space-y-1.5">
      <p
        id="lsp-connection-heading"
        className="text-[0.625rem] font-medium uppercase tracking-wider text-muted-foreground"
      >
        Connection
      </p>
      <div className="flex items-center gap-2">
        <LspStatusIcon status={status} progress={progress} />
        <span className="font-medium">{getLspConnectionLabel(status, progress)}</span>
        <span className="ml-auto font-mono text-[0.625rem] text-muted-foreground">
          {lspLanguage}
        </span>
      </div>
      {reason ? <p className="text-pretty text-muted-foreground">{reason}</p> : null}
      {setupHint ? <p className="text-pretty text-muted-foreground">{setupHint}</p> : null}
    </section>
  );
}

function ActiveProgress({
  view,
}: {
  view: Extract<ReturnType<typeof getLspProgressView>, { kind: "active" }>;
}) {
  return (
    <>
      <div className="flex items-start gap-2">
        <IconLoader2
          className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-spin text-blue-500"
          aria-hidden
        />
        <div className="min-w-0 space-y-1">
          <p className="text-pretty font-medium">{view.title}</p>
          {view.message ? (
            <p className="text-pretty text-muted-foreground">{view.message}</p>
          ) : null}
        </div>
      </div>
      {view.percentage !== null ? (
        <div className="flex items-center gap-2">
          <Progress
            value={view.percentage}
            aria-label={`${view.title}: ${view.percentage}%`}
            data-testid="lsp-work-progress-bar"
            className="flex-1"
          />
          <span className="w-8 text-right font-mono text-[0.625rem] tabular-nums">
            {view.percentage}%
          </span>
        </div>
      ) : (
        <p className="text-muted-foreground">The server did not provide a percentage.</p>
      )}
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-muted-foreground">
        <span className="tabular-nums">Elapsed {view.elapsed}</span>
        {view.concurrentCount > 1 ? <span>{view.concurrentCount} work items active</span> : null}
      </div>
      <p className="flex gap-1.5 text-pretty text-amber-700 dark:text-amber-300">
        <IconAlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden />
        Cross-file definitions and references may be incomplete while this work is active.
      </p>
    </>
  );
}

function ProjectProgress({
  status,
  progress,
  lspLanguage,
}: {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string;
}) {
  const tracked = progress.active[0]?.startedAt ?? progress.initializingSince;
  const now = useLspLiveNow(tracked !== null);
  const view = getLspProgressView(status, progress, now, lspLanguage);

  return (
    <section
      aria-labelledby="lsp-project-progress-heading"
      className="space-y-2"
      data-testid="lsp-project-progress"
      data-lsp-progress-kind={view.kind}
      data-lsp-initialization-stage={view.kind === "initializing" ? view.stage : undefined}
      aria-live="polite"
    >
      <p
        id="lsp-project-progress-heading"
        className="text-[0.625rem] font-medium uppercase tracking-wider text-muted-foreground"
      >
        Project analysis
      </p>
      {view.kind === "active" ? <ActiveProgress view={view} /> : null}
      {view.kind === "initializing" ? (
        <>
          <div className="flex items-start gap-2">
            {view.stage === "long_running" ? (
              <IconAlertTriangle
                className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500"
                aria-hidden
              />
            ) : (
              <IconLoader2
                className="mt-0.5 h-3.5 w-3.5 shrink-0 animate-spin text-blue-500"
                aria-hidden
              />
            )}
            <div className="space-y-1">
              <p className="font-medium">{view.title}</p>
              <p className="text-pretty text-muted-foreground">{view.description}</p>
            </div>
          </div>
          <p className="text-muted-foreground tabular-nums">Elapsed {view.elapsed}</p>
          <p
            className={
              view.stage === "long_running"
                ? "text-pretty text-amber-700 dark:text-amber-300"
                : "text-pretty text-muted-foreground"
            }
          >
            {view.guidance}
          </p>
        </>
      ) : null}
      {view.kind === "completed" ? (
        <>
          <p className="font-medium">{view.title}</p>
          <p className="text-pretty text-muted-foreground">
            {view.workTitle}
            {view.message ? ` — ${view.message}` : ""}
          </p>
          <p className="text-pretty text-muted-foreground">
            This confirms only the reported work item, not full project indexing.
          </p>
        </>
      ) : null}
      {view.kind === "idle" || view.kind === "waiting" ? (
        <>
          <p className="font-medium">{view.title}</p>
          <p className="text-pretty text-muted-foreground">{view.description}</p>
        </>
      ) : null}
    </section>
  );
}

export function LspProgressDetails({
  status,
  progress,
  lspLanguage,
  onToggle,
  touch,
}: {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string;
  onToggle: () => void;
  touch: boolean;
}) {
  const action = getLspLifecycleAction(status);
  return (
    <div className="space-y-3" data-testid="lsp-progress-details">
      <ConnectionDetails status={status} lspLanguage={lspLanguage} progress={progress} />
      <div className="border-t" />
      <ProjectProgress status={status} progress={progress} lspLanguage={lspLanguage} />
      <Button
        type="button"
        variant={action.label === "Stop" ? "outline" : "default"}
        className={touch ? "h-11 w-full" : "w-full"}
        onClick={onToggle}
        disabled={!action.enabled}
        data-testid="lsp-lifecycle-action"
        data-lsp-action={action.label.toLowerCase()}
      >
        {action.label} language server
      </Button>
    </div>
  );
}

type StatusTriggerProps = {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string;
  open: boolean;
  touch: boolean;
} & Omit<ComponentProps<typeof Button>, "children">;

function StatusTrigger({
  status,
  progress,
  lspLanguage,
  open,
  touch,
  ...triggerProps
}: StatusTriggerProps) {
  const label = `Language server: ${getLspConnectionLabel(status, progress)}. Open status`;
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      className={touch ? "h-11 w-11" : "h-8 w-8"}
      data-testid="lsp-status-button"
      data-lsp-state={status.state}
      data-lsp-language={lspLanguage}
      data-lsp-progress-active={
        progress.active.length > 0 || progress.initializingSince !== null ? "true" : "false"
      }
      aria-label={label}
      aria-haspopup="dialog"
      aria-expanded={open}
      {...triggerProps}
    >
      <LspStatusIcon status={status} progress={progress} />
    </Button>
  );
}

type LspStatusButtonProps = {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string | null;
  onToggle: () => void;
};

export type LspStatusDetailsProps = {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string;
  onToggle: () => void;
};

export function LspStatusPopoverContent({
  status,
  progress,
  lspLanguage,
  onToggle,
  align = "end",
  side,
}: LspStatusDetailsProps & Pick<ComponentProps<typeof PopoverContent>, "align" | "side">) {
  return (
    <PopoverContent
      align={align}
      side={side}
      sideOffset={8}
      className="w-80 gap-0 p-0"
      data-testid="lsp-status-popover"
      aria-label="Language server status"
    >
      <div className="border-b px-3 py-2.5">
        <p className="font-medium">Language server</p>
        <p className="text-muted-foreground">Connection and project analysis</p>
      </div>
      <div className="p-3">
        <LspProgressDetails
          status={status}
          progress={progress}
          lspLanguage={lspLanguage}
          onToggle={onToggle}
          touch={false}
        />
      </div>
    </PopoverContent>
  );
}

function LspStatusPopover({
  status,
  progress,
  lspLanguage,
  onToggle,
}: LspStatusButtonProps & { lspLanguage: string }) {
  const [open, setOpen] = useState(false);
  const tooltip = `Language server: ${getLspConnectionLabel(status, progress)}`;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip open={open ? false : undefined}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <StatusTrigger
              status={status}
              progress={progress}
              lspLanguage={lspLanguage}
              open={open}
              touch={false}
            />
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{tooltip}</TooltipContent>
      </Tooltip>
      <LspStatusPopoverContent
        status={status}
        progress={progress}
        lspLanguage={lspLanguage}
        onToggle={onToggle}
      />
    </Popover>
  );
}

function LspStatusDrawer({
  status,
  progress,
  lspLanguage,
  onToggle,
}: LspStatusButtonProps & { lspLanguage: string }) {
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <StatusTrigger
        status={status}
        progress={progress}
        lspLanguage={lspLanguage}
        open={open}
        touch
        onClick={() => setOpen(true)}
      />
      <DrawerContent
        data-testid="lsp-status-drawer"
        className="max-h-[80dvh] flex flex-col overflow-hidden"
      >
        <DrawerHeader className="flex flex-row items-center justify-between border-b py-2">
          <div className="min-w-0 text-left">
            <DrawerTitle>Language server</DrawerTitle>
            <DrawerDescription>Connection and project analysis</DrawerDescription>
          </div>
          <DrawerClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="h-11 w-11"
              aria-label="Close language server status"
              data-testid="lsp-status-drawer-close"
            >
              <IconX className="h-4 w-4" />
            </Button>
          </DrawerClose>
        </DrawerHeader>
        <div className="min-h-0 flex-1 overflow-y-auto p-4" data-vaul-no-drag>
          <LspProgressDetails
            status={status}
            progress={progress}
            lspLanguage={lspLanguage}
            onToggle={onToggle}
            touch
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

export function LspStatusButton(props: LspStatusButtonProps) {
  const useDrawer = useTouchDrawer();
  if (!props.lspLanguage) return null;
  if (useDrawer) return <LspStatusDrawer {...props} lspLanguage={props.lspLanguage} />;
  return <LspStatusPopover {...props} lspLanguage={props.lspLanguage} />;
}
