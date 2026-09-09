"use client";

import { useTranslation } from "react-i18next";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import AppLink from "@/components/routing/app-link";
import type { KubernetesSession } from "@/lib/types/http-kubernetes";
import {
  formatCreatedAt,
  shortId,
  taskHref,
  translateTaskLink,
  workspaceLabel,
} from "./kubernetes-session-utils";
import {
  SessionDetails,
  SessionFailureReason,
  SessionIdentity,
  SessionStatus,
} from "./kubernetes-session-presentation";

export function MobileSessionList({ sessions }: { sessions: KubernetesSession[] }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3" data-testid="kubernetes-mobile-session-list">
      {sessions.map((session) => (
        <AppLink
          key={session.session_id}
          href={taskHref(session.task_id)}
          aria-label={translateTaskLink(session, t)}
          data-testid="kubernetes-session-task-link"
          className="block min-h-11 min-w-0 cursor-pointer space-y-3 rounded-md border p-3 text-foreground transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <SessionIdentity session={session} />
          <SessionStatus session={session} />
          <SessionDetails session={session} />
        </AppLink>
      ))}
    </div>
  );
}

export function DesktopSessionTable({ sessions }: { sessions: KubernetesSession[] }) {
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
