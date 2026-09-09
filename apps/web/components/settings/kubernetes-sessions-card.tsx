"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconLoader2 } from "@tabler/icons-react";
import AppLink from "@/components/routing/app-link";
import { useKubernetesSessions } from "@/hooks/domains/settings/use-kubernetes-settings";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { formatDateTime } from "@/lib/i18n/formats";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { settingsActionClassName } from "./settings-control";

const POD_STATUS_LABEL_KEYS: Record<string, string> = {
  running: "executors:kubernetesStatusRunning",
  waiting: "executors:kubernetesStatusWaiting",
  terminated: "executors:kubernetesStatusTerminated",
  unknown: "executors:kubernetesStatusUnknown",
  pending: "executors:kubernetesStatusPending",
  succeeded: "executors:kubernetesStatusSucceeded",
  failed: "executors:kubernetesStatusFailed",
};

const RETENTION_LABEL_KEYS: Record<string, string> = {
  active: "executors:kubernetesRetentionActive",
  retained: "executors:kubernetesRetentionRetained",
  terminating: "executors:kubernetesRetentionTerminating",
  terminal: "executors:kubernetesRetentionTerminal",
  missing: "executors:kubernetesRetentionMissing",
  unknown: "executors:kubernetesRetentionUnknown",
};

const SESSION_STATE_LABEL_KEYS: Record<string, string> = {
  created: "executors:kubernetesSessionCreated",
  starting: "executors:kubernetesSessionStarting",
  running: "executors:kubernetesSessionRunning",
  idle: "executors:kubernetesSessionIdle",
  waiting_for_input: "executors:kubernetesSessionWaitingForInput",
  completed: "executors:kubernetesSessionCompleted",
  failed: "executors:kubernetesSessionFailed",
  cancelled: "executors:kubernetesSessionCancelled",
  unknown: "executors:kubernetesStatusUnknown",
};

type KubernetesSessionsState = ReturnType<typeof useKubernetesSessions>;

export function KubernetesSessionsCard({ state }: { state: KubernetesSessionsState }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const errorMessage =
    state.error instanceof Error && state.error.message
      ? state.error.message
      : t("executors:kubernetesSessionsFailed");
  return (
    <SettingsCard className="min-w-0 overflow-hidden" data-testid="kubernetes-sessions-card">
      <SettingsCardHeader
        title={t("executors:kubernetesActiveSessions")}
        description={t("executors:kubernetesActiveSessionsDescription")}
        actions={
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void state.refresh().catch(() => undefined)}
            disabled={state.loading}
            className={settingsActionClassName("w-full cursor-pointer md:w-auto")}
          >
            {state.loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {t("executors:refresh")}
          </Button>
        }
      />
      <CardContent className="min-w-0">
        {!state.error && <SessionGuidance />}
        {Boolean(state.error) && (
          <p className="break-words text-sm text-destructive">{errorMessage}</p>
        )}
        {!state.error && !state.loading && state.sessions.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {t("executors:kubernetesNoActiveSessions")}
          </p>
        )}
        {!state.error &&
          state.sessions.length > 0 &&
          (isMobile ? (
            <MobileSessionList sessions={state.sessions} t={t} />
          ) : (
            <DesktopSessionTable sessions={state.sessions} />
          ))}
      </CardContent>
    </SettingsCard>
  );
}

function SessionGuidance() {
  const { t } = useTranslation();
  return (
    <p
      className="mb-3 break-words text-xs text-muted-foreground"
      data-testid="kubernetes-session-guidance"
    >
      {t("executors:kubernetesActiveSessionsGuidance")}
    </p>
  );
}

function MobileSessionList({ sessions, t }: { sessions: KubernetesSession[]; t: TFunction }) {
  return (
    <div className="space-y-3" data-testid="kubernetes-mobile-session-list">
      {sessions.map((session) => (
        <AppLink
          key={session.session_id}
          href={taskHref(session.task_id)}
          aria-label={translateTaskLink(session, t)}
          data-testid="kubernetes-session-task-link"
          className="block min-h-11 min-w-0 space-y-3 rounded-md border p-3 text-foreground transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <SessionIdentity session={session} />
          <SessionStatus session={session} />
          <SessionDetails session={session} />
        </AppLink>
      ))}
    </div>
  );
}

function DesktopSessionTable({ sessions }: { sessions: KubernetesSession[] }) {
  const { t } = useTranslation();
  return (
    <div className="max-w-full overflow-x-auto overscroll-x-contain">
      <Table data-testid="kubernetes-sessions-table">
        <TableHeader>
          <TableRow>
            <TableHead>{t("executors:task")}</TableHead>
            <TableHead>{t("executors:session")}</TableHead>
            <TableHead>{t("executors:kubernetesPod")}</TableHead>
            <TableHead>{t("executors:status")}</TableHead>
            <TableHead>{t("executors:kubernetesWorkspace")}</TableHead>
            <TableHead>{t("executors:kubernetesCreated")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sessions.map((session) => (
            <TableRow key={session.session_id}>
              <TableCell className="font-mono text-xs">
                <AppLink
                  href={taskHref(session.task_id)}
                  aria-label={translateTaskLink(session, t)}
                  data-testid="kubernetes-session-task-link"
                  className="cursor-pointer break-all underline-offset-2 hover:underline"
                >
                  {shortId(session.task_id)}
                </AppLink>
              </TableCell>
              <TableCell className="font-mono text-xs">{shortId(session.session_id)}</TableCell>
              <TableCell className="max-w-56 break-all font-mono text-xs">
                {session.pod_name || "-"}
              </TableCell>
              <TableCell
                className="max-w-64 whitespace-normal"
                data-testid="kubernetes-session-status"
              >
                <div className="space-y-1">
                  <SessionStatus session={session} />
                  <SessionFailureReason session={session} />
                </div>
              </TableCell>
              <TableCell className="text-xs">{workspaceLabel(session.workspace_kind, t)}</TableCell>
              <TableCell className="whitespace-nowrap text-xs">
                {formatCreatedAt(session.created_at)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function SessionIdentity({ session }: { session: KubernetesSession }) {
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

function SessionStatus({ session }: { session: KubernetesSession }) {
  const { t } = useTranslation();
  const podStatus = podStatusValue(session);
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
        <p>{t("executors:kubernetesPodStateValue", { value: podStatusLabel(podStatus, t) })}</p>
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

function SessionDetails({ session }: { session: KubernetesSession }) {
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

function SessionFailureReason({ session }: { session: KubernetesSession }) {
  return session.failure_reason ? (
    <p className="break-words text-xs text-destructive">{session.failure_reason}</p>
  ) : null;
}

function workspaceLabel(value: string | undefined, t: TFunction): string {
  const keys: Record<string, string> = {
    managed_pvc: "executors:kubernetesWorkspaceManagedPvc",
    empty_dir: "executors:kubernetesWorkspaceEmptyDir",
    existing_claim: "executors:kubernetesWorkspaceExistingClaim",
  };
  return value && keys[value] ? t(keys[value]) : value || "-";
}

function podStatusLabel(value: string, t: TFunction): string {
  const key = POD_STATUS_LABEL_KEYS[value.toLowerCase()];
  return key ? t(key) : t("executors:kubernetesStatusUnknown");
}

function retentionLabel(value: string, t: TFunction): string {
  return t(RETENTION_LABEL_KEYS[value] ?? RETENTION_LABEL_KEYS.unknown);
}

function sessionStateLabel(value: string | undefined, t: TFunction): string {
  const state = normalizedState(value);
  return t(SESSION_STATE_LABEL_KEYS[state] ?? SESSION_STATE_LABEL_KEYS.unknown);
}

function normalizedState(value?: string): string {
  return value?.trim().toLowerCase() || "unknown";
}

function podStatusValue(session: KubernetesSession): string {
  if (
    session.container_state?.toLowerCase() === "terminated" &&
    session.pod_phase?.toLowerCase() === "succeeded"
  ) {
    return session.pod_phase;
  }
  return session.container_state || session.pod_phase || "unknown";
}

function translateTaskLink(session: KubernetesSession, t: TFunction): string {
  return t("executors:kubernetesOpenTask", { task: session.task_id });
}

function taskHref(taskId: string): string {
  return `/t/${encodeURIComponent(taskId)}`;
}

function shortId(value: string): string {
  return value.slice(0, 8);
}

function formatCreatedAt(value?: string): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : formatDateTime(date);
}
