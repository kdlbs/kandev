"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconLoader2 } from "@tabler/icons-react";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { settingsActionClassName } from "./settings-control";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { useKubernetesSessions } from "@/hooks/domains/settings/use-kubernetes-settings";
import { formatDateTime } from "@/lib/i18n/formats";
import { i18n, t as translate } from "@/lib/i18n";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";

const STATUS_LABEL_KEYS: Record<string, string> = {
  running: "executors:kubernetesStatusRunning",
  waiting: "executors:kubernetesStatusWaiting",
  terminated: "executors:kubernetesStatusTerminated",
  unknown: "executors:kubernetesStatusUnknown",
  pending: "executors:kubernetesStatusPending",
  succeeded: "executors:kubernetesStatusSucceeded",
  failed: "executors:kubernetesStatusFailed",
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
            <MobileSessionList sessions={state.sessions} />
          ) : (
            <DesktopSessionTable sessions={state.sessions} />
          ))}
      </CardContent>
    </SettingsCard>
  );
}

function MobileSessionList({ sessions }: { sessions: KubernetesSession[] }) {
  return (
    <div className="space-y-3" data-testid="kubernetes-mobile-session-list">
      {sessions.map((session) => (
        <div key={session.session_id} className="min-w-0 space-y-3 rounded-md border p-3">
          <SessionIdentity session={session} />
          <SessionStatus session={session} />
          <SessionDetails session={session} />
        </div>
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
              <TableCell className="font-mono text-xs">{shortId(session.task_id)}</TableCell>
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
              <TableCell className="text-xs">{workspaceLabel(session.workspace_kind)}</TableCell>
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
  const status =
    session.container_state?.toLowerCase() === "terminated" &&
    session.pod_phase?.toLowerCase() === "succeeded"
      ? session.pod_phase
      : session.container_state || session.pod_phase || "unknown";
  const running = status.toLowerCase() === "running";
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant={running ? "default" : "secondary"}>{statusLabel(status)}</Badge>
      {session.restarts > 0 && <RestartCount count={session.restarts} />}
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
          value: workspaceLabel(session.workspace_kind),
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

function workspaceLabel(value?: string): string {
  const keys: Record<string, string> = {
    managed_pvc: "executors:kubernetesWorkspaceManagedPvc",
    empty_dir: "executors:kubernetesWorkspaceEmptyDir",
    existing_claim: "executors:kubernetesWorkspaceExistingClaim",
  };
  return value && keys[value] ? translationLabel(keys[value]) : value || "-";
}

function statusLabel(value: string): string {
  const key = STATUS_LABEL_KEYS[value.toLowerCase()];
  return key ? translationLabel(key, value) : value;
}

function translationLabel(key: string, fallback?: string): string {
  return i18n.exists(key) ? translate(key) : (fallback ?? key);
}

function shortId(value: string): string {
  return value.slice(0, 8);
}

function formatCreatedAt(value?: string): string {
  if (!value) return "-";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : formatDateTime(date);
}
