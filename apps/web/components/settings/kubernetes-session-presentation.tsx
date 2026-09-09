"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import {
  normalizedState,
  mainContainerStateValue,
  podPhaseValue,
  podStatusLabel,
  retentionLabel,
  sessionStateLabel,
  formatCreatedAt,
  workspaceLabel,
} from "./kubernetes-session-utils";

export function SessionIdentity({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-1.5">
      <p className="break-all font-mono text-sm">{session.pod_name || "-"}</p>
      <SessionIdentityValue label={t("executors:task")} value={session.task_id} />
      <SessionIdentityValue label={t("executors:session")} value={session.session_id} />
    </div>
  );
}

function SessionIdentityValue({ label, value }: { label: string; value: string }) {
  return (
    <p className="min-w-0 break-all font-mono text-xs text-muted-foreground">
      <span className="font-sans">{label}: </span>
      {value}
    </p>
  );
}

export function SessionStatusSummary({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  const sessionState = normalizedState(session.session_state);
  const retentionState = normalizedState(session.retention_state);
  const podPhase = podPhaseValue(session);
  const mainContainerState = mainContainerStateValue(session);
  return (
    <div className="min-w-0 space-y-1.5" data-testid="kubernetes-session-status-summary">
      <div className="flex flex-nowrap items-center gap-1.5">
        <Badge variant={sessionState === "running" ? "default" : "secondary"}>
          {sessionStateLabel(sessionState, t)}
        </Badge>
        <Badge variant={retentionState === "retained" ? "outline" : "secondary"}>
          {retentionLabel(retentionState, t)}
        </Badge>
      </div>
      <div className="flex min-w-0 flex-wrap items-center gap-1 text-xs text-muted-foreground md:flex-nowrap md:whitespace-nowrap">
        <span>
          {t("executors:kubernetesPodCompactValue", { value: podStatusLabel(podPhase, t) })}
        </span>
        <span aria-hidden="true">·</span>
        <span>
          {t("executors:kubernetesContainerCompactValue", {
            value: podStatusLabel(mainContainerState, t),
          })}
        </span>
        {session.restarts > 0 && (
          <>
            <span aria-hidden="true">·</span>
            <RestartCount count={session.restarts} />
          </>
        )}
      </div>
    </div>
  );
}

export function SessionRuntimeDetails({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  const retentionState = normalizedState(session.retention_state);
  const podPhase = podPhaseValue(session);
  const mainContainerState = mainContainerStateValue(session);
  return (
    <div
      className="grid gap-x-6 gap-y-1 text-xs text-muted-foreground md:grid-cols-2 xl:grid-cols-4"
      data-testid="kubernetes-session-runtime-details"
    >
      <p>
        {t("executors:kubernetesSessionStateValue", {
          value: sessionStateLabel(session.session_state, t),
        })}
      </p>
      <p>
        {t("executors:kubernetesRetentionStateValue", {
          value: retentionLabel(retentionState, t),
        })}
      </p>
      <p>{t("executors:kubernetesPodStateValue", { value: podStatusLabel(podPhase, t) })}</p>
      <p>
        {t("executors:kubernetesContainerStateValue", {
          value: podStatusLabel(mainContainerState, t),
        })}
      </p>
      <p>{t("executors:kubernetesRestarts", { count: session.restarts })}</p>
      <p>
        {t("executors:kubernetesWorkspaceValue", {
          value: workspaceLabel(session.workspace_kind, t),
        })}
      </p>
      <p>{t("executors:kubernetesCreatedValue", { value: formatCreatedAt(session.created_at) })}</p>
      <RequestSummary session={session} showLabel />
      <SessionFailureReason session={session} />
    </div>
  );
}

export function RequestSummary({
  session,
  showLabel = false,
}: {
  session: KubernetesSession;
  showLabel?: boolean;
}) {
  const { t } = useTranslation();
  const requests = session.main_container_requests;
  const values = [
    requests?.cpu ? t("executors:kubernetesRequestCpu", { value: requests.cpu }) : null,
    requests?.memory ? t("executors:kubernetesRequestMemory", { value: requests.memory }) : null,
  ].filter((value): value is string => Boolean(value));
  return (
    <p
      className="min-w-0 text-xs text-muted-foreground"
      data-testid="kubernetes-session-request-summary"
    >
      {showLabel && <span>{t("executors:kubernetesMainContainerRequests")}: </span>}
      {values.length > 0 ? values.join(" · ") : t("executors:kubernetesRequestsUnspecifiedValue")}
    </p>
  );
}

function RestartCount({ count }: { count: number }) {
  const { t } = useTranslation();
  return <span>{t("executors:kubernetesRestarts", { count })}</span>;
}

export function SessionDetails({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
      <p>
        {t("executors:kubernetesWorkspaceValue", {
          value: workspaceLabel(session.workspace_kind, t),
        })}
      </p>
      <p>{t("executors:kubernetesCreatedValue", { value: formatCreatedAt(session.created_at) })}</p>
      <SessionFailureReason session={session} />
    </div>
  );
}

export function SessionFailureReason({ session }: { session: KubernetesSession }) {
  return session.failure_reason ? (
    <p className="break-words text-xs text-destructive">{session.failure_reason}</p>
  ) : null;
}
