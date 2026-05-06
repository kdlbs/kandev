"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconBox, IconCopy, IconLoader, IconLoader2, IconTrash } from "@tabler/icons-react";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { toast } from "sonner";

import {
  fetchTaskEnvironmentLive,
  resetTaskEnvironment,
  type ContainerLiveStatus,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import { ApiError } from "@/lib/api/client";
import { getExecutorStatusIcon } from "@/lib/executor-icons";
import { TaskResetEnvConfirmDialog } from "./task-reset-env-confirm-dialog";
import {
  getEnvironmentStatusSnapshot,
  resolveExecutorEnvironmentStatus,
  type StatusTone,
  type EnvironmentStatusSnapshot,
} from "./executor-environment-status";
import {
  isPreparingPhase,
  PrepareStatusSection,
  usePrepareSummary,
} from "./executor-prepare-status";

type ExecutorSettingsButtonProps = {
  taskId?: string | null;
  sessionId?: string | null;
  disabled?: boolean;
};

const ACTIVE_POLL_INTERVAL_MS = 3000;
const BACKGROUND_POLL_INTERVAL_MS = 7000;

/**
 * Owns the env+container fetch/poll lifecycle and the reset action so the
 * popover component stays small. Polls more quickly while the popover is open
 * and less frequently while closed so the toolbar icon can still reflect
 * externally stopped/restarted containers.
 */
function useTaskEnvironment(taskId: string | null | undefined, active: boolean) {
  const [env, setEnv] = useState<TaskEnvironment | null>(null);
  const [container, setContainer] = useState<ContainerLiveStatus | null>(null);
  const [loading, setLoading] = useState(false);
  const [isResetting, setIsResetting] = useState(false);
  const inFlight = useRef(false);
  const lastStatusRef = useRef<EnvironmentStatusSnapshot | null>(null);
  // hasLoadedRef tracks "have we ever fetched successfully" so the spinner
  // only shows on the first open. Keeping it in a ref instead of state means
  // `loadEnv` doesn't depend on `env` — without that, every successful fetch
  // creates a new `env` reference, forces a new `loadEnv` identity, and the
  // polling effect's cleanup+rerun fires immediately, turning the 3-second
  // poll into a tight loop.
  const hasLoadedRef = useRef(false);

  useEffect(() => {
    hasLoadedRef.current = false;
    lastStatusRef.current = null;
    setEnv(null);
    setContainer(null);
    setLoading(false);
  }, [taskId]);

  const updateState = useCallback(
    (nextEnv: TaskEnvironment | null, nextContainer: ContainerLiveStatus | null) => {
      const nextStatus = getEnvironmentStatusSnapshot(nextEnv, nextContainer);
      maybeNotifyEnvironmentStatus(lastStatusRef.current, nextStatus);
      lastStatusRef.current = nextStatus;
      setEnv(nextEnv);
      setContainer(nextContainer);
    },
    [],
  );

  const loadEnv = useCallback(async () => {
    if (!taskId || inFlight.current) return;
    inFlight.current = true;
    setLoading((prev) => prev || (active && !hasLoadedRef.current));
    try {
      const data = await fetchTaskEnvironmentLive(taskId);
      hasLoadedRef.current = true;
      updateState(data.environment, data.container ?? null);
    } catch (err) {
      // Only treat 404 as "no environment yet" — a transient 500 / auth /
      // network error should leave the last-known view in place rather than
      // erase a valid environment and disable the Reset action.
      if (err instanceof ApiError && err.status === 404) {
        hasLoadedRef.current = true;
        updateState(null, null);
      }
    } finally {
      inFlight.current = false;
      setLoading(false);
    }
  }, [active, taskId, updateState]);

  useEffect(() => {
    if (!taskId) return;
    void loadEnv();
    const intervalMs = active ? ACTIVE_POLL_INTERVAL_MS : BACKGROUND_POLL_INTERVAL_MS;
    const interval = window.setInterval(() => void loadEnv(), intervalMs);
    return () => window.clearInterval(interval);
  }, [active, taskId, loadEnv]);

  const reset = useCallback(
    async ({ pushBranch }: { pushBranch: boolean }) => {
      if (!taskId) return false;
      setIsResetting(true);
      try {
        await resetTaskEnvironment(taskId, { push_branch: pushBranch });
        toast.success("Environment reset");
        lastStatusRef.current = getEnvironmentStatusSnapshot(null, null);
        setEnv(null);
        setContainer(null);
        return true;
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Unknown error";
        toast.error(`Reset failed: ${msg}`);
        return false;
      } finally {
        setIsResetting(false);
      }
    },
    [taskId],
  );

  const status = useMemo(
    () => (env ? resolveExecutorEnvironmentStatus(env, container) : null),
    [env, container],
  );

  return { env, container, loading, isResetting, reset, status };
}

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
          {/* Borderless info trigger: not a button — just an icon + status dot.
              tabIndex makes it focusable so keyboard users can still surface
              the popover via focus. */}
          <span
            tabIndex={disabled ? -1 : 0}
            role="button"
            aria-haspopup="dialog"
            aria-label={ariaLabel}
            data-testid="executor-settings-button"
            data-disabled={disabled || undefined}
            className="relative inline-flex h-7 items-center gap-1 rounded px-1.5 text-muted-foreground hover:text-foreground hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
          >
            <ExecutorButtonIcon executorType={executorType} preparing={isPreparing} />
            <ExecutorStatusDot status={status} loading={loading} />
          </span>
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

function maybeNotifyEnvironmentStatus(
  prev: EnvironmentStatusSnapshot | null,
  next: EnvironmentStatusSnapshot,
) {
  if (!prev || prev.key === next.key) return;
  if (prev.tone === "running" && next.tone !== "running") {
    toast.error("Executor environment stopped", {
      description: `Current state: ${next.label}`,
    });
  } else if (prev.tone !== "running" && next.tone === "running") {
    toast.success("Executor environment running");
  }
}

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

function EnvironmentInfo({
  env,
  container,
  loading,
}: {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  loading: boolean;
}) {
  if (loading && !env) {
    return (
      <div className="flex items-center justify-center py-6 text-muted-foreground">
        <IconLoader className="h-4 w-4 animate-spin" />
      </div>
    );
  }

  if (!env) {
    return (
      <div className="px-3 py-4 text-muted-foreground">
        <p className="font-medium text-foreground">No environment yet</p>
        <p className="text-xs mt-1">
          An environment is created when you start a session on this task.
        </p>
      </div>
    );
  }

  return (
    <div className="px-3 pb-1.5 space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium text-foreground">{formatExecutorType(env.executor_type)}</span>
        <StatusBadge env={env} container={container} />
      </div>
      <EnvironmentFields env={env} container={container} />
    </div>
  );
}

function StatusBadge({
  env,
  container,
}: {
  env: TaskEnvironment;
  container: ContainerLiveStatus | null;
}) {
  // For container-backed envs the live state is the source of truth; for the
  // others fall back to the recorded TaskEnvironment.status.
  const { label, tone } = resolveExecutorEnvironmentStatus(env, container);
  const className = TONE_CLASSES[tone];
  return (
    <Badge variant="outline" className={`text-[10px] uppercase ${className}`}>
      {label}
    </Badge>
  );
}

const TONE_CLASSES: Record<StatusTone, string> = {
  running: "border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300",
  stopped: "border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",
  warn: "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  error: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",
  neutral: "border-muted text-muted-foreground",
};

const DOT_CLASSES: Record<StatusTone, string> = {
  running: "bg-green-500",
  stopped: "bg-zinc-500",
  warn: "bg-amber-500",
  error: "bg-red-500",
  neutral: "bg-muted-foreground",
};

function EnvironmentFields({
  env,
  container,
}: {
  env: TaskEnvironment;
  container: ContainerLiveStatus | null;
}) {
  const fields = useMemo(() => buildFields(env, container), [env, container]);
  if (fields.length === 0) {
    return <p className="text-xs text-muted-foreground">No resource details available.</p>;
  }
  return (
    <dl className="space-y-1 text-xs">
      {fields.map((f) => (
        <Field key={f.label} label={f.label} value={f.value} copy={f.copy} />
      ))}
    </dl>
  );
}

function Field({ label, value, copy }: { label: string; value: string; copy?: boolean }) {
  return (
    <div className="flex items-start gap-2">
      <dt className="text-muted-foreground min-w-[80px]">{label}</dt>
      <dd className="flex-1 flex items-center gap-1 break-all font-mono">
        <span className="flex-1">{value}</span>
        {copy && (
          <button
            type="button"
            className="cursor-pointer text-muted-foreground hover:text-foreground"
            aria-label={`Copy ${label}`}
            onClick={() => {
              void navigator.clipboard
                .writeText(value)
                .then(() => toast.success(`${label} copied`));
            }}
          >
            <IconCopy className="h-3 w-3" />
          </button>
        )}
      </dd>
    </div>
  );
}

type FieldRow = { label: string; value: string; copy?: boolean };

function buildFields(env: TaskEnvironment, container: ContainerLiveStatus | null): FieldRow[] {
  const rows: FieldRow[] = [];

  if (env.worktree_path) {
    rows.push({ label: "Worktree", value: env.worktree_path, copy: true });
  }
  if (env.worktree_branch) {
    rows.push({ label: "Branch", value: env.worktree_branch, copy: true });
  }

  if (env.container_id) {
    const short = env.container_id.slice(0, 12);
    rows.push({ label: "Container", value: short, copy: true });
    // Use `sh` rather than `bash` — user-built images may only ship
    // /bin/sh (busybox/alpine/etc.), and the bootstrap entrypoint already
    // assumes sh-only.
    rows.push({
      label: "Shell",
      value: `docker exec -it ${short} sh`,
      copy: true,
    });
    if (container?.started_at && container.state === "running") {
      rows.push({ label: "Uptime", value: formatUptime(container.started_at) });
    }
  }

  if (env.sandbox_id) {
    rows.push({ label: "Sprite", value: env.sandbox_id, copy: true });
  }

  return rows;
}

function formatUptime(startedAt: string): string {
  const startedMs = Date.parse(startedAt);
  if (Number.isNaN(startedMs)) return startedAt;
  const elapsedSec = Math.max(0, Math.floor((Date.now() - startedMs) / 1000));
  if (elapsedSec < 60) return `${elapsedSec}s`;
  const min = Math.floor(elapsedSec / 60);
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  return `${hr}h ${min % 60}m`;
}

function formatExecutorType(type: string): string {
  switch (type) {
    case "local_pc":
    case "worktree":
      return "Local (worktree)";
    case "local_docker":
      return "Local Docker";
    case "sprites":
      return "Sprites sandbox";
    case "remote_docker":
      return "Remote Docker";
    default:
      return type || "Unknown executor";
  }
}
