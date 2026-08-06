"use client";

import { forwardRef, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconBraces, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";
import { useTaskLsp } from "@/hooks/domains/lsp/use-task-lsp";
import { useAppStore } from "@/components/state-provider";
import { formatLspElapsed } from "@/lib/lsp/lsp-progress-view";
import {
  deriveTaskLspViewModel,
  type TaskLspLanguageView,
  type TaskLspViewModel,
  type TaskLspVisualState,
} from "@/lib/lsp/task-lsp-view-model";
import type { TaskLspPolicy } from "@/lib/types/http-lsp";
import { TaskLspLanguageRow } from "./task-lsp-language-row";
import { TaskLspRestartDialog } from "./task-lsp-restart-dialog";

const STATE_KEYS: Record<TaskLspVisualState, string> = {
  error: "lsp:taskStateError",
  server_work: "lsp:taskStateServerWork",
  initializing: "lsp:taskStateInitializing",
  installing: "lsp:taskStateInstalling",
  starting: "lsp:taskStateStarting",
  queued: "lsp:taskStateQueued",
  stopping: "lsp:taskStateStopping",
  ready: "lsp:taskStateReady",
  unsupported: "lsp:taskStateUnsupported",
  waiting: "lsp:taskStateWaiting",
  detected: "lsp:taskStateDetected",
  configured: "lsp:taskStateConfigured",
  stopped: "lsp:taskStateStopped",
  off: "lsp:taskStateOff",
};

function useLiveNow(enabled: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!enabled) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, [enabled]);
  return Math.max(now, Date.now());
}

function useTaskLspView(taskId: string | null, forceVisibleLanguage?: string | null) {
  const controller = useTaskLsp(taskId);
  const hiddenLanguages = useAppStore((state) => state.userSettings.lspStatusHiddenLanguages ?? []);
  const tracksElapsed = controller.languages.some(
    (language) =>
      language.progress.length > 0 ||
      Boolean(language.initialize_started_at) ||
      Boolean(language.process_started_at),
  );
  const now = useLiveNow(tracksElapsed);
  const view = useMemo(
    () =>
      deriveTaskLspViewModel(controller.languages, now, {
        hiddenLanguages,
        forceVisibleLanguage,
      }),
    [controller.languages, forceVisibleLanguage, hiddenLanguages, now],
  );
  return { controller, view, now };
}

function orderedRows(rows: TaskLspLanguageView[], focusLanguage?: string | null) {
  if (!focusLanguage) return rows;
  return [...rows].sort((left, right) => {
    if (left.language === focusLanguage) return -1;
    if (right.language === focusLanguage) return 1;
    return left.label.localeCompare(right.label);
  });
}

function useExpandedLanguages(focusLanguage?: string | null) {
  const [expandedLanguages, setExpandedLanguages] = useState<Set<string>>(
    () => new Set(focusLanguage ? [focusLanguage] : []),
  );

  useEffect(() => {
    if (!focusLanguage) return;
    setExpandedLanguages((current) => {
      if (current.has(focusLanguage)) return current;
      const next = new Set(current);
      next.add(focusLanguage);
      return next;
    });
  }, [focusLanguage]);

  const setExpanded = (language: string, open: boolean) => {
    setExpandedLanguages((current) => {
      const next = new Set(current);
      if (open) next.add(language);
      else next.delete(language);
      return next;
    });
  };
  return { expandedLanguages, setExpanded };
}

function TaskLspEmptyState({ hiddenCount }: { hiddenCount: number }) {
  const { t } = useTranslation();
  const hasHiddenLanguages = hiddenCount > 0;
  return (
    <div className="space-y-2 rounded-md border border-dashed p-3 text-sm text-muted-foreground">
      <p>
        {t(hasHiddenLanguages ? "lsp:allTaskLanguageServersHidden" : "lsp:noTaskLanguageServers")}
      </p>
      {hasHiddenLanguages ? (
        <a
          href="/settings/general/editors"
          className="inline-flex min-h-11 items-center font-medium text-foreground underline underline-offset-4 hover:text-primary"
        >
          {t("lsp:manageStatusVisibility")}
        </a>
      ) : null}
    </div>
  );
}

export function TaskLspDisclosure({
  taskId,
  touch,
  focusLanguage,
}: {
  taskId: string;
  touch: boolean;
  focusLanguage?: string | null;
}) {
  const { t } = useTranslation();
  const { controller, view, now } = useTaskLspView(taskId, focusLanguage);
  const [restartLanguage, setRestartLanguage] = useState<string | null>(null);
  const { expandedLanguages, setExpanded } = useExpandedLanguages(focusLanguage);
  const rows = orderedRows(view.rows, focusLanguage);
  const restartRow = rows.find((row) => row.language === restartLanguage) ?? null;
  const run = (action: () => Promise<unknown>) => void action().catch(() => undefined);
  const setPolicy = (language: string, policy: TaskLspPolicy) =>
    run(() => controller.setPolicy(language, policy));

  return (
    <div
      className="min-w-0 shrink-0 space-y-3 [overflow-wrap:anywhere]"
      data-testid="task-lsp-disclosure"
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-semibold tracking-[-0.01em]">{t("lsp:taskLanguageServers")}</h2>
          <p className="text-xs text-muted-foreground">{t("lsp:taskLanguageServersDescription")}</p>
        </div>
        {controller.capacity.limit > 0 ? (
          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
            {t("lsp:capacitySummary", {
              active: controller.capacity.active,
              limit: controller.capacity.limit,
              queued: controller.capacity.queued,
            })}
          </span>
        ) : null}
      </div>

      {controller.error ? (
        <div
          role="alert"
          className="flex min-w-0 gap-2 rounded-md border border-destructive/25 bg-destructive/5 p-3 text-sm text-destructive [overflow-wrap:anywhere]"
        >
          <IconAlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span>{controller.error}</span>
        </div>
      ) : null}
      {controller.loading && rows.length === 0 ? (
        <div className="flex min-h-20 items-center justify-center gap-2 text-sm text-muted-foreground">
          <IconLoader2 className="size-4 animate-spin" aria-hidden />
          {t("lsp:loadingTaskLanguageServers")}
        </div>
      ) : null}
      {controller.loaded && rows.length === 0 ? (
        <TaskLspEmptyState hiddenCount={view.hiddenCount} />
      ) : null}
      <div className="space-y-2">
        {rows.map((row) => (
          <TaskLspLanguageRow
            key={row.language}
            row={row}
            now={now}
            touch={touch}
            open={expandedLanguages.has(row.language)}
            pending={controller.pending[row.language]}
            onOpenChange={(open) => setExpanded(row.language, open)}
            onStart={() => run(() => controller.start(row.language))}
            onStop={() => run(() => controller.stop(row.language))}
            onRestart={() => setRestartLanguage(row.language)}
            onSetPolicy={(policy) => setPolicy(row.language, policy)}
          />
        ))}
      </div>
      <TaskLspRestartDialog
        language={restartRow?.label ?? null}
        pending={restartRow ? controller.pending[restartRow.language] === "restart" : false}
        onOpenChange={(open) => {
          if (!open) setRestartLanguage(null);
        }}
        onConfirm={() => {
          if (!restartRow) return;
          setRestartLanguage(null);
          run(() => controller.restart(restartRow.language));
        }}
      />
    </div>
  );
}

type TaskLspControlPlacement = "status-bar" | "task-topbar" | "editor-toolbar";

type TaskLspControlProps = {
  taskId: string | null;
  placement: TaskLspControlPlacement;
  language?: string | null;
  hideWhenIrrelevant?: boolean;
  touch?: boolean;
  externalOpen?: boolean;
  onOpenExternal?: (language?: string) => void;
};

function compactText(
  view: TaskLspViewModel,
  selected: TaskLspLanguageView | undefined,
  t: (key: string, values?: Record<string, unknown>) => string,
) {
  if (selected) {
    const state = selected.work?.title ?? t(STATE_KEYS[selected.state]);
    const elapsed = selected.elapsedMs === null ? "" : ` · ${formatLspElapsed(selected.elapsedMs)}`;
    return `${selected.label} · ${state}${elapsed}`;
  }
  const compact = view.aggregate.compact;
  if (compact.kind === "empty") return t("lsp:taskLanguageServersShort");
  if (compact.kind === "single") {
    const state = compact.workTitle ?? t(STATE_KEYS[compact.state]);
    const elapsed = compact.elapsedMs === null ? "" : ` · ${formatLspElapsed(compact.elapsedMs)}`;
    return `${compact.language} · ${state}${elapsed}`;
  }
  if (compact.errorCount > 0) {
    return t("lsp:aggregateErrors", { count: compact.errorCount });
  }
  return t("lsp:aggregateRunning", { count: compact.runningCount });
}

function controlTestId(placement: TaskLspControlPlacement) {
  if (placement === "status-bar") return "app-status-lsp";
  if (placement === "editor-toolbar") return "lsp-status-button";
  return "task-lsp-control";
}

function triggerSizeClass(placement: TaskLspControlPlacement, touch: boolean): string {
  if (touch) return "h-11 w-11";
  if (placement === "task-topbar") return "h-7 px-2";
  return "h-8 w-8";
}

function CompactIcon({ view }: { view: TaskLspViewModel }) {
  if (view.aggregate.errorCount > 0) {
    return <IconAlertTriangle className="size-3.5 shrink-0 text-amber-500" aria-hidden />;
  }
  if (view.aggregate.workCount > 0) {
    return <IconLoader2 className="size-3.5 shrink-0 animate-spin text-blue-500" aria-hidden />;
  }
  return <IconBraces className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />;
}

type TriggerButtonProps = {
  placement: TaskLspControlPlacement;
  view: TaskLspViewModel;
  language?: string | null;
  state?: TaskLspVisualState;
  summary: string;
  open: boolean;
  touch: boolean;
  onClick?: () => void;
};

const TriggerButton = forwardRef<HTMLButtonElement, TriggerButtonProps>(function TriggerButton(
  { placement, view, language, state, summary, open, touch, onClick },
  ref,
) {
  const { t } = useTranslation();
  const iconOnly = placement === "editor-toolbar" || touch;
  const common = {
    ref,
    type: "button" as const,
    "aria-label": t("lsp:openTaskControl", { summary }),
    "aria-haspopup": "dialog" as const,
    "aria-expanded": open,
    "data-testid": controlTestId(placement),
    "data-lsp-placement": placement,
    "data-lsp-language": language || undefined,
    "data-lsp-state": state,
    onClick,
  };
  if (placement === "status-bar") {
    return (
      <button
        {...common}
        className="inline-flex h-full max-w-80 min-w-0 cursor-pointer items-center gap-1.5 rounded-sm px-1 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
      >
        <CompactIcon view={view} />
        <span className="truncate text-muted-foreground tabular-nums">{summary}</span>
      </button>
    );
  }
  return (
    <Button
      {...common}
      variant="ghost"
      size={iconOnly ? "icon-sm" : "sm"}
      className={cn("cursor-pointer gap-1.5", triggerSizeClass(placement, touch))}
    >
      <CompactIcon view={view} />
      {!iconOnly ? <span className="max-w-48 truncate text-xs">{summary}</span> : null}
    </Button>
  );
});

type ControlSurfaceProps = {
  taskId: string;
  placement: TaskLspControlPlacement;
  language?: string | null;
  view: TaskLspViewModel;
  state?: TaskLspVisualState;
  summary: string;
  touch: boolean;
};

function ExternalControlSurface({
  placement,
  language,
  view,
  state,
  summary,
  touch,
  externalOpen,
  onOpenExternal,
}: Omit<ControlSurfaceProps, "taskId"> & {
  externalOpen: boolean;
  onOpenExternal: (language?: string) => void;
}) {
  const { t } = useTranslation();
  const [tooltipOpen, setTooltipOpen] = useState(false);
  return (
    <Tooltip open={!externalOpen && tooltipOpen} onOpenChange={setTooltipOpen}>
      <TooltipTrigger asChild>
        <TriggerButton
          placement={placement}
          view={view}
          language={language}
          state={state}
          summary={summary}
          open={externalOpen}
          touch={touch}
          onClick={() => onOpenExternal(language ?? undefined)}
        />
      </TooltipTrigger>
      <TooltipContent>{t("lsp:openTaskControl", { summary })}</TooltipContent>
    </Tooltip>
  );
}

function PopoverControlSurface({
  taskId,
  placement,
  language,
  view,
  state,
  summary,
  touch,
}: ControlSurfaceProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip open={!open && tooltipOpen} onOpenChange={setTooltipOpen}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <TriggerButton
              placement={placement}
              view={view}
              language={language}
              state={state}
              summary={summary}
              open={open}
              touch={touch}
            />
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("lsp:openTaskControl", { summary })}</TooltipContent>
      </Tooltip>
      <PopoverContent
        align="end"
        side={placement === "status-bar" ? "top" : "bottom"}
        sideOffset={8}
        className="max-h-[min(42rem,calc(100dvh-2rem))] w-[min(30rem,calc(100vw-1rem))] overflow-y-auto p-3"
        data-testid="task-lsp-surface"
        aria-label={t("lsp:taskLanguageServers")}
      >
        <TaskLspDisclosure taskId={taskId} touch={false} focusLanguage={language} />
      </PopoverContent>
    </Popover>
  );
}

export function TaskLspControl({
  taskId,
  placement,
  language,
  hideWhenIrrelevant = false,
  touch = false,
  externalOpen = false,
  onOpenExternal,
}: TaskLspControlProps) {
  const { t } = useTranslation();
  const { controller, view } = useTaskLspView(taskId, language);
  const selected = language ? view.rows.find((row) => row.language === language) : undefined;
  const relevant = view.relevantRows.length > 0 || controller.loading || Boolean(controller.error);
  if (!taskId || (hideWhenIrrelevant && !relevant)) return null;
  const summary = compactText(view, selected, t);
  if (onOpenExternal) {
    return (
      <ExternalControlSurface
        placement={placement}
        language={language}
        view={view}
        state={selected?.state}
        summary={summary}
        touch={touch}
        externalOpen={externalOpen}
        onOpenExternal={onOpenExternal}
      />
    );
  }
  return (
    <PopoverControlSurface
      taskId={taskId}
      placement={placement}
      language={language}
      view={view}
      state={selected?.state}
      summary={summary}
      touch={touch}
    />
  );
}
