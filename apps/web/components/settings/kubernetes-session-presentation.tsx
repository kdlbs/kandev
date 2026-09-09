"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import {
  normalizedState,
  mainContainerStateValue,
  podPhaseValue,
  podStatusLabel,
  podStatusValue,
  retentionLabel,
  sessionStateLabel,
  formatCreatedAt,
  workspaceLabel,
} from "./kubernetes-session-utils";

export function SessionIdentity({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 space-y-2">
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{t("executors:kubernetesPod")}</p>
        <p className="break-all font-mono text-sm">{session.pod_name || "-"}</p>
      </div>
      <div className="grid min-w-0 gap-2 sm:grid-cols-2">
        <SessionIdentityValue label={t("executors:task")} value={session.task_id} />
        <SessionIdentityValue label={t("executors:session")} value={session.session_id} />
      </div>
    </div>
  );
}

function SessionIdentityValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="break-all font-mono text-xs">{value}</p>
    </div>
  );
}

export function SessionStatus({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  const podStatus = podStatusValue(session);
  const podPhase = podPhaseValue(session);
  const mainContainerState = mainContainerStateValue(session);
  const retentionState = normalizedState(session.retention_state);
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={retentionState === "retained" ? "outline" : "secondary"}>
          {retentionLabel(retentionState, t)}
        </Badge>
        <Badge variant={podStatus.toLowerCase() === "running" ? "default" : "secondary"}>
          {podStatusLabel(podStatus, t)}
        </Badge>
        {session.restarts > 0 && <RestartCount count={session.restarts} />}
      </div>
      <div className="space-y-1 text-xs text-muted-foreground">
        <p>
          {t("executors:kubernetesSessionStateValue", {
            value: sessionStateLabel(session.session_state, t),
          })}
        </p>
        <p>{t("executors:kubernetesPodStateValue", { value: podStatusLabel(podPhase, t) })}</p>
        <p>
          {t("executors:kubernetesContainerStateValue", {
            value: podStatusLabel(mainContainerState, t),
          })}
        </p>
        <p>
          {t("executors:kubernetesRetentionStateValue", {
            value: retentionLabel(retentionState, t),
          })}
        </p>
      </div>
      <RequestSummary session={session} />
    </div>
  );
}

function RequestSummary({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  const requests = session.main_container_requests;
  return (
    <div className="space-y-1 text-xs text-muted-foreground">
      <p>{t("executors:kubernetesMainContainerRequests")}</p>
      {requests?.cpu && (
        <p className="break-all">{t("executors:kubernetesRequestCpu", { value: requests.cpu })}</p>
      )}
      {requests?.memory && (
        <p className="break-all">
          {t("executors:kubernetesRequestMemory", { value: requests.memory })}
        </p>
      )}
      {!requests?.cpu && !requests?.memory && <p>{t("executors:kubernetesRequestsUnspecified")}</p>}
    </div>
  );
}

function RestartCount({ count }: { count: number }) {
  const { t } = useTranslation();
  return (
    <span className="text-xs text-muted-foreground">
      {t("executors:kubernetesRestarts", { count })}
    </span>
  );
}

export function SessionDetails({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1 text-xs text-muted-foreground">
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
