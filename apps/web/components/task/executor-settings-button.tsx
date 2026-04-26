"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  IconBox,
  IconCopy,
  IconLoader,
  IconRefresh,
  IconTrash,
} from "@tabler/icons-react";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Button } from "@kandev/ui/button";
import { toast } from "sonner";

import {
  fetchTaskEnvironment,
  resetTaskEnvironment,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import { TaskResetEnvConfirmDialog } from "./task-reset-env-confirm-dialog";

type ExecutorSettingsButtonProps = {
  taskId?: string | null;
  disabled?: boolean;
};

export function ExecutorSettingsButton({ taskId, disabled }: ExecutorSettingsButtonProps) {
  const [open, setOpen] = useState(false);
  const [env, setEnv] = useState<TaskEnvironment | null>(null);
  const [loading, setLoading] = useState(false);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [isResetting, setIsResetting] = useState(false);

  const loadEnv = useCallback(async () => {
    if (!taskId) {
      setEnv(null);
      return;
    }
    setLoading(true);
    try {
      const data = await fetchTaskEnvironment(taskId);
      setEnv(data);
    } catch {
      // 404 when there's no environment yet — normal state, not an error
      setEnv(null);
    } finally {
      setLoading(false);
    }
  }, [taskId]);

  useEffect(() => {
    if (open) {
      void loadEnv();
    }
  }, [open, loadEnv]);

  const handleReset = useCallback(
    async ({ pushBranch }: { pushBranch: boolean }) => {
      if (!taskId) return;
      setIsResetting(true);
      try {
        await resetTaskEnvironment(taskId, { push_branch: pushBranch });
        toast.success("Environment reset");
        setEnv(null);
        setResetDialogOpen(false);
        setOpen(false);
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Unknown error";
        toast.error(`Reset failed: ${msg}`);
      } finally {
        setIsResetting(false);
      }
    },
    [taskId],
  );

  if (!taskId) return null;

  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer px-2"
            disabled={disabled}
            data-testid="executor-settings-button"
            aria-label="Executor settings"
          >
            <IconBox className="h-4 w-4" />
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="end"
          className="w-[340px] p-0 text-sm"
          data-testid="executor-settings-popover"
        >
          <EnvironmentInfo env={env} loading={loading} />
          <div className="border-t border-border p-2 flex items-center justify-between gap-2">
            <Button
              variant="ghost"
              size="sm"
              className="cursor-pointer text-xs"
              onClick={() => void loadEnv()}
              disabled={loading}
            >
              <IconRefresh className="h-3.5 w-3.5 mr-1" /> Refresh
            </Button>
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
        </PopoverContent>
      </Popover>

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

function EnvironmentInfo({
  env,
  loading,
}: {
  env: TaskEnvironment | null;
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
    <div className="p-3 space-y-3">
      <div>
        <div className="flex items-center justify-between">
          <span className="font-medium text-foreground">{formatExecutorType(env.executor_type)}</span>
          <span className="text-xs text-muted-foreground uppercase tracking-wide">{env.status}</span>
        </div>
      </div>
      <EnvironmentFields env={env} />
    </div>
  );
}

function EnvironmentFields({ env }: { env: TaskEnvironment }) {
  const fields = useMemo(() => buildFields(env), [env]);
  if (fields.length === 0) {
    return <p className="text-xs text-muted-foreground">No resource details available.</p>;
  }
  return (
    <dl className="space-y-1.5 text-xs">
      {fields.map((f) => (
        <Field key={f.label} label={f.label} value={f.value} copy={f.copy} />
      ))}
    </dl>
  );
}

function Field({
  label,
  value,
  copy,
}: {
  label: string;
  value: string;
  copy?: boolean;
}) {
  return (
    <div className="flex items-start gap-2">
      <dt className="text-muted-foreground min-w-[90px]">{label}</dt>
      <dd className="flex-1 flex items-center gap-1 break-all font-mono">
        <span className="flex-1">{value}</span>
        {copy && (
          <button
            type="button"
            className="cursor-pointer text-muted-foreground hover:text-foreground"
            aria-label={`Copy ${label}`}
            onClick={() => {
              void navigator.clipboard.writeText(value).then(() => toast.success(`${label} copied`));
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

function buildFields(env: TaskEnvironment): FieldRow[] {
  const rows: FieldRow[] = [];

  if (env.worktree_path) {
    rows.push({ label: "Worktree", value: env.worktree_path, copy: true });
  }
  if (env.worktree_branch) {
    rows.push({ label: "Branch", value: env.worktree_branch, copy: true });
  }

  if (env.container_id) {
    rows.push({ label: "Container", value: env.container_id.slice(0, 12), copy: true });
    rows.push({
      label: "Shell",
      value: `docker exec -it ${env.container_id.slice(0, 12)} bash`,
      copy: true,
    });
  }

  if (env.sandbox_id) {
    rows.push({ label: "Sprite", value: env.sandbox_id, copy: true });
  }

  if (env.agent_execution_id) {
    rows.push({ label: "Execution", value: env.agent_execution_id, copy: true });
  }

  rows.push({ label: "Environment ID", value: env.id, copy: true });

  return rows;
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
