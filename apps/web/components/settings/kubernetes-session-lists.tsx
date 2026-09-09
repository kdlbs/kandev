"use client";

import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconChevronDown } from "@tabler/icons-react";
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
  RequestSummary,
  SessionDetails,
  SessionIdentity,
  SessionRuntimeDetails,
  SessionStatusSummary,
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
          className="block min-h-11 min-w-0 cursor-pointer space-y-2.5 rounded-md border p-3 text-foreground transition-colors hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <SessionIdentity session={session} />
          <SessionStatusSummary session={session} />
          <RequestSummary session={session} showLabel />
          <SessionDetails session={session} />
        </AppLink>
      ))}
    </div>
  );
}

export function DesktopSessionTable({ sessions }: { sessions: KubernetesSession[] }) {
  const { t } = useTranslation();
  const [expandedSessionId, setExpandedSessionId] = useState<string | null>(null);
  return (
    <div className="max-w-full overflow-x-auto overscroll-x-contain">
      <Table className="min-w-[880px]" data-testid="kubernetes-sessions-table">
        <TableHeader>
          <TableRow>
            <TableHead>{t("executors:task")}</TableHead>
            <TableHead>{t("executors:session")}</TableHead>
            <TableHead>{t("executors:kubernetesPod")}</TableHead>
            <TableHead>{t("executors:status")}</TableHead>
            <TableHead>{t("executors:kubernetesRequests")}</TableHead>
            <TableHead className="text-right">{t("executors:kubernetesCreated")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sessions.map((session) => {
            const expanded = expandedSessionId === session.session_id;
            return (
              <Fragment key={session.session_id}>
                <TableRow data-testid="kubernetes-session-row">
                  <TableCell className="whitespace-nowrap py-2 font-mono text-xs">
                    <AppLink
                      href={taskHref(session.task_id)}
                      aria-label={translateTaskLink(session, t)}
                      data-testid="kubernetes-session-task-link"
                      className="cursor-pointer underline-offset-2 hover:underline"
                    >
                      {shortId(session.task_id)}
                    </AppLink>
                  </TableCell>
                  <TableCell className="whitespace-nowrap py-2 font-mono text-xs">
                    {shortId(session.session_id)}
                  </TableCell>
                  <TableCell className="max-w-56 py-2">
                    <p className="truncate font-mono text-xs">{session.pod_name || "-"}</p>
                    <p className="mt-1 whitespace-nowrap text-xs text-muted-foreground">
                      {workspaceLabel(session.workspace_kind, t)}
                    </p>
                  </TableCell>
                  <TableCell className="py-2" data-testid="kubernetes-session-status">
                    <div className="flex min-w-max items-start gap-1">
                      <SessionStatusSummary session={session} />
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-expanded={expanded}
                        aria-label={t(
                          expanded
                            ? "executors:kubernetesHideSessionDetails"
                            : "executors:kubernetesShowSessionDetails",
                        )}
                        className="cursor-pointer"
                        onClick={() => setExpandedSessionId(expanded ? null : session.session_id)}
                      >
                        <IconChevronDown
                          aria-hidden="true"
                          className={`transition-transform ${expanded ? "rotate-180" : ""}`}
                        />
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-nowrap py-2">
                    <RequestSummary session={session} />
                  </TableCell>
                  <TableCell className="whitespace-nowrap py-2 text-right text-xs">
                    {formatCreatedAt(session.created_at)}
                  </TableCell>
                </TableRow>
                {expanded && (
                  <TableRow data-testid="kubernetes-session-details-row">
                    <TableCell colSpan={6} className="bg-muted/20 py-3">
                      <SessionRuntimeDetails session={session} />
                    </TableCell>
                  </TableRow>
                )}
              </Fragment>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}
