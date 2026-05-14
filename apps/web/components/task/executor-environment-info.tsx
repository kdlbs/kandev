"use client";

import { useMemo } from "react";
import { IconCopy, IconLoader } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { toast } from "sonner";

import {
  type ContainerLiveStatus,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import { resolveExecutorEnvironmentStatus, type StatusTone } from "./executor-environment-status";

const TONE_CLASSES: Record<StatusTone, string> = {
  running: "border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300",
  stopped: "border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",
  warn: "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  error: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",
  neutral: "border-muted text-muted-foreground",
};

export function EnvironmentInfo({
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
    <div className="px-3 pt-2.5 pb-1.5 space-y-1.5">
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
