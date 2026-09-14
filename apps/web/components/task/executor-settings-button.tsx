"use client";

import { forwardRef, useCallback, useState } from "react";
import { IconBox, IconLoader2 } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
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

import { getExecutorStatusIcon } from "@/lib/executor-icons";
import { TaskResetEnvConfirmDialog } from "./task-reset-env-confirm-dialog";
import type { StatusTone } from "./executor-environment-status";
import { usePrepareSummary } from "@/hooks/domains/session/use-prepare-summary";
import { useTaskEnvironment } from "@/hooks/domains/session/use-task-environment";
import { isPreparingPhase } from "@/lib/prepare/summarize";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { PrepareStatusSection } from "./executor-prepare-status";
import { ExecutorEnvironmentDisclosure } from "./executor-environment-disclosure";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

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
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const prepare = usePrepareSummary(sessionId ?? null);
  const isPreparing = isPreparingPhase(prepare.phase);
  // Promote the foreground polling cadence while preparing so the icon flips
  // to "ready" without the user hovering over the trigger.
  const {
    env,
    container,
    ssh,
    kubernetes,
    kubernetesLoaded,
    kubernetesError,
    loading,
    refreshing,
    isResetting,
    reset,
    refresh,
    status,
  } = useTaskEnvironment(taskId, sessionId, open || isPreparing);
  const hasWorktreePath = Boolean(env?.worktree_path);

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
  const content = (
    <>
      <PrepareStatusSection summary={prepare} />
      <ExecutorEnvironmentDisclosure
        env={env}
        container={container}
        ssh={ssh}
        kubernetes={kubernetes}
        kubernetesLoaded={kubernetesLoaded}
        kubernetesError={kubernetesError}
        loading={loading}
        refreshing={refreshing}
        isResetting={isResetting}
        touch={usesTouchDrawer}
        onRefresh={refresh}
        onReset={() => setResetDialogOpen(true)}
      />
    </>
  );

  return (
    <ExecutorDisclosureSurface
      touch={usesTouchDrawer}
      open={open}
      onOpenChange={setOpen}
      content={content}
      trigger={{
        ariaLabel,
        disabled,
        executorType,
        loading,
        preparing: isPreparing,
        status,
      }}
      resetDialog={{
        open: resetDialogOpen,
        onOpenChange: setResetDialogOpen,
        hasWorktreePath,
        isResetting,
        onConfirm: handleReset,
      }}
    />
  );
}

type ExecutorDisclosureSurfaceProps = {
  touch: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  content: React.ReactNode;
  trigger: Omit<ExecutorTriggerProps, "expanded" | "touch">;
  resetDialog: React.ComponentProps<typeof TaskResetEnvConfirmDialog>;
};

function ExecutorDisclosureSurface({
  touch,
  open,
  onOpenChange,
  content,
  trigger,
  resetDialog,
}: ExecutorDisclosureSurfaceProps) {
  const { t } = useTranslation();
  if (touch) {
    return (
      <>
        <Drawer open={open} onOpenChange={onOpenChange}>
          <DrawerTrigger asChild>
            <ExecutorTrigger {...trigger} touch expanded={open} />
          </DrawerTrigger>
          <DrawerContent
            data-testid="executor-settings-drawer"
            className="max-h-[78dvh] overflow-hidden pb-[env(safe-area-inset-bottom)]"
          >
            <DrawerHeader className="flex flex-row items-center justify-between border-b px-4 py-3">
              <div className="min-w-0 text-left">
                <DrawerTitle className="text-sm">{t("task:executorSettings")}</DrawerTitle>
                <DrawerDescription className="truncate text-xs">
                  {trigger.status
                    ? t("task:environmentStateCurrent", { state: trigger.status.label })
                    : t("task:executorEnvironmentIsUnavailable")}
                </DrawerDescription>
              </div>
              <DrawerClose asChild>
                <Button variant="ghost" size="sm" className="min-h-11 cursor-pointer">
                  {t("common:close")}
                </Button>
              </DrawerClose>
            </DrawerHeader>
            <div className="min-h-0 overflow-y-auto overscroll-contain">{content}</div>
          </DrawerContent>
        </Drawer>
        <TaskResetEnvConfirmDialog {...resetDialog} />
      </>
    );
  }
  return (
    <>
      <HoverCard open={open} onOpenChange={onOpenChange} openDelay={150} closeDelay={250}>
        <HoverCardTrigger asChild>
          <ExecutorTrigger {...trigger} expanded={open} />
        </HoverCardTrigger>
        <HoverCardContent
          align="start"
          className="max-h-[70vh] w-[348px] max-w-[calc(100vw-2rem)] overflow-y-auto overscroll-contain rounded-xl border-border/80 p-0 text-sm shadow-xl"
          data-testid="executor-settings-popover"
        >
          {content}
        </HoverCardContent>
      </HoverCard>
      <TaskResetEnvConfirmDialog {...resetDialog} />
    </>
  );
}

type ExecutorTriggerProps = {
  ariaLabel: string;
  disabled?: boolean;
  executorType: string | null;
  loading: boolean;
  preparing: boolean;
  status: { label: string; tone: StatusTone } | null;
  touch?: boolean;
  expanded: boolean;
} & Omit<React.ComponentPropsWithoutRef<typeof Button>, "children">;

const ExecutorTrigger = forwardRef<HTMLButtonElement, ExecutorTriggerProps>(
  function ExecutorTrigger(
    {
      ariaLabel,
      disabled,
      executorType,
      loading,
      preparing,
      status,
      touch = false,
      expanded,
      ...triggerProps
    },
    ref,
  ) {
    return (
      <Button
        {...triggerProps}
        ref={ref}
        type="button"
        variant="ghost"
        size="sm"
        disabled={disabled}
        aria-haspopup="dialog"
        aria-expanded={expanded}
        aria-label={ariaLabel}
        data-testid="executor-settings-button"
        className={
          touch
            ? "relative h-11 w-11 cursor-pointer p-0 text-muted-foreground hover:text-foreground"
            : "relative h-7 cursor-pointer gap-1 px-1.5 text-muted-foreground hover:text-foreground"
        }
      >
        <ExecutorButtonIcon
          executorType={executorType}
          preparing={preparing}
          hasError={status?.tone === "error"}
        />
        <ExecutorStatusDot status={status} loading={loading} />
      </Button>
    );
  },
);

function computeAriaLabel(
  preparing: boolean,
  status: { label: string; tone: StatusTone } | null,
): string {
  if (preparing) return t("task:executorSettingsPreparing");
  if (status) return t("task:executorSettingsWithStatus", { status: status.label });
  return t("task:executorSettings");
}

function ExecutorButtonIcon({
  executorType,
  preparing,
  hasError,
}: {
  executorType: string | null;
  preparing: boolean;
  hasError: boolean;
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
  const { Icon, testId } = getExecutorStatusIcon(executorType, hasError);
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
  const { t } = useTranslation();
  const tone = status?.tone ?? "neutral";
  const label = status?.label ?? "not created";
  return (
    <span
      aria-hidden="true"
      title={t("task:environment", { label })}
      data-testid="executor-status-indicator"
      className={`absolute right-1 top-1 h-2.5 w-2.5 rounded-full border border-background ${DOT_CLASSES[tone]} ${
        loading && !status ? "animate-pulse" : ""
      }`}
    />
  );
}
