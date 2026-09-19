"use client";

import { useMemo } from "react";
import { IconCopy, IconLoader } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { toast } from "@/lib/toast/sonner";

import {
  type ContainerLiveStatus,
  type SSHLiveStatus,
  type TaskEnvironment,
} from "@/lib/api/domains/task-environment-api";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { formatDateTime } from "@/lib/i18n/formats";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { getExecutorIcon } from "@/lib/executor-icons";
import { resolveExecutorEnvironmentStatus, type StatusTone } from "./executor-environment-status";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

const TONE_CLASSES: Record<StatusTone, string> = {
  running: "border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300",
  stopped: "border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300",
  warn: "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300",
  error: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300",
  neutral: "border-muted text-muted-foreground",
};

const KubernetesPodIcon = getExecutorIcon("k8s");
const KUBERNETES_POD_KEY = "executors:kubernetesPod";

export function EnvironmentInfo({
  env,
  container,
  ssh,
  kubernetes,
  kubernetesLoaded = false,
  kubernetesError = null,
  loading,
  kubernetesActions,
}: {
  env: TaskEnvironment | null;
  container: ContainerLiveStatus | null;
  ssh?: SSHLiveStatus | null;
  kubernetes?: KubernetesSession | null;
  kubernetesLoaded?: boolean;
  kubernetesError?: string | null;
  loading: boolean;
  kubernetesActions?: React.ReactNode;
}) {
  const { t } = useTranslation();
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
        <p className="font-medium text-foreground">{t("task:noEnvironmentYet")}</p>
        <p className="text-xs mt-1">{t("task:anEnvironmentIsCreatedWhenYou")}</p>
      </div>
    );
  }

  if (env.executor_type === "k8s") {
    return (
      <KubernetesEnvironmentSummary
        env={env}
        container={container}
        session={kubernetes ?? null}
        loaded={kubernetesLoaded}
        error={kubernetesError}
        actions={kubernetesActions}
      />
    );
  }

  return (
    <div className="px-3 pt-2.5 pb-1.5 space-y-1.5">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium text-foreground">{formatExecutorType(env.executor_type)}</span>
        <StatusBadge
          env={env}
          container={container}
          kubernetes={kubernetes ?? null}
          kubernetesLoaded={kubernetesLoaded}
          kubernetesError={kubernetesError}
        />
      </div>
      <EnvironmentFields
        env={env}
        container={container}
        ssh={ssh ?? null}
        kubernetes={kubernetes ?? null}
        kubernetesLoaded={kubernetesLoaded}
        kubernetesError={kubernetesError}
      />
    </div>
  );
}

function StatusBadge({
  env,
  container,
  kubernetes,
  kubernetesLoaded,
  kubernetesError,
}: {
  env: TaskEnvironment;
  container: ContainerLiveStatus | null;
  kubernetes: KubernetesSession | null;
  kubernetesLoaded: boolean;
  kubernetesError: string | null;
}) {
  // For container-backed envs the live state is the source of truth; for the
  // others fall back to the recorded TaskEnvironment.status.
  const { label, tone } = resolveExecutorEnvironmentStatus(
    env,
    container,
    env.executor_type === "k8s"
      ? { session: kubernetes, loaded: kubernetesLoaded, error: kubernetesError }
      : undefined,
  );
  const className = TONE_CLASSES[tone];
  return (
    <Badge variant="outline" className={`h-6 rounded-full px-2.5 text-[11px] ${className}`}>
      {label}
    </Badge>
  );
}

function KubernetesEnvironmentSummary({
  env,
  container,
  session,
  loaded,
  error,
  actions,
}: {
  env: TaskEnvironment;
  container: ContainerLiveStatus | null;
  session: KubernetesSession | null;
  loaded: boolean;
  error: string | null;
  actions?: React.ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <section className="space-y-3 p-3" data-testid="kubernetes-environment-summary">
      <div className="flex items-start gap-2.5">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-primary/15 bg-primary/8 text-primary shadow-sm">
          <KubernetesPodIcon className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1 pt-0.5">
          <h3 className="text-sm font-semibold tracking-tight text-foreground">
            {formatExecutorType(env.executor_type)} {t(KUBERNETES_POD_KEY)}
          </h3>
          <div className="mt-1">
            <StatusBadge
              env={env}
              container={container}
              kubernetes={session}
              kubernetesLoaded={loaded}
              kubernetesError={error}
            />
          </div>
        </div>
        {actions}
      </div>

      {session ? (
        <>
          {session.pod_name ? (
            <div className="rounded-lg border border-border/70 bg-muted/25 px-3 py-2.5">
              <p className="text-[11px] font-medium text-muted-foreground">
                {t(KUBERNETES_POD_KEY)}
              </p>
              <div className="mt-0.5 flex min-w-0 items-center gap-2">
                <span className="min-w-0 flex-1 break-all font-mono text-xs text-foreground">
                  {session.pod_name}
                </span>
                <CopyButton label={t(KUBERNETES_POD_KEY)} value={session.pod_name} />
              </div>
            </div>
          ) : null}
          <dl className="grid grid-cols-2 gap-x-4 gap-y-3">
            {session.pod_phase ? (
              <KubernetesFact label={t("task:status")} value={session.pod_phase} />
            ) : null}
            {session.container_state ? (
              <KubernetesFact label={t("task:container")} value={session.container_state} />
            ) : null}
            <KubernetesFact
              label={t("task:restarts")}
              value={String(session.restarts)}
              testId="kubernetes-restart-count"
              tabular
            />
            {session.workspace_kind ? (
              <KubernetesFact
                label={t("task:workspaceMode")}
                value={session.workspace_kind}
                technical
              />
            ) : null}
            {session.created_at ? (
              <KubernetesFact
                className="col-span-2"
                label={t("task:created")}
                value={formatDateTime(session.created_at)}
              />
            ) : null}
          </dl>
          {session.failure_reason ? (
            <div className="rounded-lg border border-red-500/25 bg-red-500/8 px-3 py-2.5">
              <p className="text-[11px] font-medium text-red-700 dark:text-red-300">
                {t("task:error")}
              </p>
              <p className="mt-1 break-words text-xs text-foreground">{session.failure_reason}</p>
            </div>
          ) : null}
        </>
      ) : (
        <p className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-muted-foreground">
          {error || (loaded ? t("task:executorEnvironmentIsUnavailable") : t("task:loadingStatus"))}
        </p>
      )}
    </section>
  );
}

function KubernetesFact({
  label,
  value,
  className = "",
  testId,
  technical = false,
  tabular = false,
}: {
  label: string;
  value: string;
  className?: string;
  testId?: string;
  technical?: boolean;
  tabular?: boolean;
}) {
  return (
    <div className={className}>
      <dt className="text-[11px] font-medium text-muted-foreground">{label}</dt>
      <dd
        data-testid={testId}
        className={`mt-0.5 break-words text-xs text-foreground ${technical ? "font-mono" : ""} ${tabular ? "tabular-nums" : ""}`}
      >
        {value}
      </dd>
    </div>
  );
}

function CopyButton({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="-mr-2 flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-[0.96]"
      aria-label={t("task:copy2", { label })}
      onClick={() => {
        void copyToClipboard(value).then((success) => {
          if (success) toast.success(t("task:copied3", { label }));
        });
      }}
    >
      <IconCopy className="h-4 w-4" />
    </button>
  );
}

function EnvironmentFields({
  env,
  container,
  ssh,
  kubernetes,
  kubernetesLoaded,
  kubernetesError,
}: {
  env: TaskEnvironment;
  container: ContainerLiveStatus | null;
  ssh: SSHLiveStatus | null;
  kubernetes: KubernetesSession | null;
  kubernetesLoaded: boolean;
  kubernetesError: string | null;
}) {
  const { t } = useTranslation();
  const fields = useMemo(
    () => buildFields(env, container, ssh, kubernetes),
    [container, env, kubernetes, ssh],
  );
  if (fields.length === 0) {
    if (env.executor_type === "k8s") {
      let message = t("task:loadingStatus");
      if (kubernetesError) message = kubernetesError;
      else if (kubernetesLoaded) message = t("task:executorEnvironmentIsUnavailable");
      return <p className="text-xs text-muted-foreground">{message}</p>;
    }
    return <p className="text-xs text-muted-foreground">{t("task:noResourceDetailsAvailable")}</p>;
  }
  return (
    <dl className="space-y-1 text-xs">
      {fields.map((f) => (
        <Field
          key={f.label}
          label={f.label}
          value={f.value}
          copy={f.copy}
          copyValue={f.copyValue}
        />
      ))}
    </dl>
  );
}

function Field({
  label,
  value,
  copy,
  copyValue,
}: {
  label: string;
  value: string;
  copy?: boolean;
  copyValue?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-start gap-2">
      <dt className="text-muted-foreground min-w-[80px]">{label}</dt>
      <dd className="flex-1 flex items-center gap-1 break-all font-mono">
        <span className="flex-1">{value}</span>
        {copy && (
          <button
            type="button"
            className="cursor-pointer text-muted-foreground hover:text-foreground"
            aria-label={t("task:copy2", { label })}
            onClick={() => {
              void copyToClipboard(copyValue ?? value).then((success) => {
                if (success) toast.success(t("task:copied3", { label }));
              });
            }}
          >
            <IconCopy className="h-3 w-3" />
          </button>
        )}
      </dd>
    </div>
  );
}

// copyValue overrides the clipboard payload when the displayed value is a
// truncated representation (fingerprint hash suffix, …) — copying the
// truncation is useless. Omit to copy `value` verbatim.
type FieldRow = { label: string; value: string; copy?: boolean; copyValue?: string };

function buildFields(
  env: TaskEnvironment,
  container: ContainerLiveStatus | null,
  ssh: SSHLiveStatus | null,
  kubernetes: KubernetesSession | null,
): FieldRow[] {
  const rows: FieldRow[] = [];

  if (env.worktree_path) {
    rows.push({ label: t("task:worktree"), value: env.worktree_path, copy: true });
  }
  if (env.worktree_branch) {
    rows.push({ label: t("task:branch"), value: env.worktree_branch, copy: true });
  }

  if (env.container_id) {
    const short = env.container_id.slice(0, 12);
    rows.push({ label: t("task:container"), value: short, copy: true });
    // Use `sh` rather than `bash` — user-built images may only ship
    // /bin/sh (busybox/alpine/etc.), and the bootstrap entrypoint already
    // assumes sh-only.
    rows.push({
      label: t("common:shell"),
      value: `docker exec -it ${short} sh`,
      copy: true,
    });
    if (container?.started_at && container.state === "running") {
      rows.push({ label: t("task:uptime"), value: formatUptime(container.started_at) });
    }
  }

  if (env.sandbox_id) {
    rows.push({ label: t("task:sprite"), value: env.sandbox_id, copy: true });
  }

  if (env.executor_type === "ssh" && ssh) {
    addSshRows(rows, ssh);
  }

  if (env.executor_type === "k8s" && kubernetes) {
    addKubernetesRows(rows, kubernetes);
  }

  return rows;
}

function addKubernetesRows(rows: FieldRow[], session: KubernetesSession) {
  if (session.pod_name) {
    rows.push({ label: t(KUBERNETES_POD_KEY), value: session.pod_name, copy: true });
  }
  if (session.pod_phase) {
    rows.push({ label: t("task:status"), value: session.pod_phase });
  }
  if (session.container_state) {
    rows.push({ label: t("task:container"), value: session.container_state });
  }
  rows.push({ label: t("task:restarts"), value: String(session.restarts) });
  if (session.workspace_kind) {
    rows.push({ label: t("task:workspaceMode"), value: session.workspace_kind });
  }
  if (session.created_at) {
    rows.push({ label: t("task:created"), value: formatDateTime(session.created_at) });
  }
  if (session.failure_reason) {
    rows.push({ label: t("task:error"), value: session.failure_reason });
  }
}

function addSshRows(rows: FieldRow[], ssh: SSHLiveStatus) {
  // user@host[:port] — matches what a user would paste into an SSH client.
  // Suppress :22 since the canonical port reads as noise.
  if (ssh.host) {
    rows.push({ label: t("task:host"), value: formatHostTarget(ssh), copy: true });
  }
  if (ssh.remote_task_dir) {
    rows.push({ label: t("task:workdir"), value: ssh.remote_task_dir, copy: true });
  }
  const agentctl = formatAgentctlSummary(ssh);
  if (agentctl) {
    rows.push({ label: t("task:agentctl"), value: agentctl });
  }
  if (ssh.fingerprint) {
    rows.push({
      label: t("task:fingerprint"),
      value: formatFingerprint(ssh.fingerprint),
      copy: true,
      copyValue: ssh.fingerprint,
    });
  }
  // Shell affordance: paste-ready ssh command that mirrors how kandev
  // connects. Helpful when the user wants to inspect the remote dir by hand.
  if (ssh.host) {
    rows.push({ label: t("common:shell"), value: formatShellCommand(ssh), copy: true });
  }
}

function formatHostTarget(ssh: SSHLiveStatus): string {
  const userPart = ssh.user ? `${ssh.user}@` : "";
  const portPart = ssh.port && ssh.port !== 22 ? `:${ssh.port}` : "";
  return `${userPart}${ssh.host ?? ""}${portPart}`;
}

function formatShellCommand(ssh: SSHLiveStatus): string {
  const userPart = ssh.user ? `${ssh.user}@` : "";
  const portPart = ssh.port && ssh.port !== 22 ? ` -p ${ssh.port}` : "";
  return `ssh${portPart} ${userPart}${ssh.host ?? ""}`;
}

// Compact one-liner for the agentctl process: pid → remote:port → local:port.
// Each is optional (e.g. a recovered session may have lost the local
// forward port mid-restore), so the helper omits gracefully. Returns ""
// when nothing is set so callers can skip the row entirely.
function formatAgentctlSummary(ssh: SSHLiveStatus): string {
  const parts: string[] = [];
  if (ssh.remote_agentctl_pid) parts.push(`pid ${ssh.remote_agentctl_pid}`);
  if (ssh.remote_agentctl_port) parts.push(`remote :${ssh.remote_agentctl_port}`);
  if (ssh.local_forward_port) parts.push(`local :${ssh.local_forward_port}`);
  return parts.join(" → ");
}

// Show the suffix only — full fingerprint is verbose, and the trust gate
// already pinned it on save. The copy button hands back the full string.
function formatFingerprint(fingerprint: string): string {
  return fingerprint.startsWith("SHA256:") ? `…${fingerprint.slice(-12)}` : fingerprint;
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
      return t("task:executorTypeLocalWorktree");
    case "local_docker":
      return t("task:executorTypeLocalDocker");
    case "sprites":
      return t("task:executorTypeSpritesSandbox");
    case "remote_docker":
      return t("task:executorTypeRemoteDocker");
    case "ssh":
      return "SSH";
    case "k8s":
      return t("executors:typeKubernetes");
    default:
      return type || t("task:executorTypeUnknown");
  }
}
