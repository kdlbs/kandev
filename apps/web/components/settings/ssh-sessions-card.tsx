"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconLoader2 } from "@tabler/icons-react";
import { listSSHSessions } from "@/lib/api/domains/ssh-api";
import type { SSHSession } from "@/lib/types/http-ssh";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { settingsActionClassName } from "@/components/settings/settings-control";

export interface SSHSessionsCardProps {
  executorId: string;
}

const REFRESH_INTERVAL_MS = 90_000;

export function SSHSessionsCard({ executorId }: SSHSessionsCardProps) {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<SSHSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Monotonic sequence used to ignore stale poll responses: a slow request
  // that resolves after a newer one (or after executorId changes) would
  // otherwise overwrite the fresh row set.
  const seqRef = useRef(0);

  const refresh = useCallback(async () => {
    const seq = ++seqRef.current;
    setLoading(true);
    setError(null);
    try {
      const rows = await listSSHSessions(executorId);
      if (seq !== seqRef.current) return; // a newer call (or executor switch) won the race
      setSessions(rows);
    } catch (e) {
      if (seq !== seqRef.current) return;
      setError(e instanceof Error ? e.message : t("executors:sshFailedToLoadSessions"));
    } finally {
      if (seq === seqRef.current) setLoading(false);
    }
  }, [executorId, t]);

  useEffect(() => {
    // Reset sequence so a previous executor's pending response can't land
    // here and pollute the new executor's data.
    seqRef.current = 0;
    refresh();
    const id = setInterval(refresh, REFRESH_INTERVAL_MS);
    return () => {
      clearInterval(id);
      // Bump the sequence to invalidate any in-flight response from the
      // unmounted instance.
      seqRef.current = -1;
    };
  }, [refresh]);

  return (
    <Card data-testid="ssh-sessions-card">
      <SettingsCardHeader
        title={t("executors:sshActiveSessions")}
        description={t("executors:sshActiveSessionsDescription")}
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={refresh}
            disabled={loading}
            data-testid="ssh-sessions-refresh"
            className={settingsActionClassName("cursor-pointer")}
          >
            {loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {t("executors:refresh")}
          </Button>
        }
      />
      <CardContent>
        <SSHSessionsBody loading={loading} error={error} sessions={sessions} />
      </CardContent>
    </Card>
  );
}

function SSHSessionsBody({
  loading,
  error,
  sessions,
}: {
  loading: boolean;
  error: string | null;
  sessions: SSHSession[];
}) {
  const { t } = useTranslation();
  if (error) {
    return (
      <p data-testid="ssh-sessions-error" className="text-sm text-red-600">
        {error}
      </p>
    );
  }
  if (sessions.length === 0 && !loading) {
    return (
      <p data-testid="ssh-sessions-empty" className="text-sm text-muted-foreground">
        {t("executors:sshNoActiveSessions")}
      </p>
    );
  }
  if (sessions.length === 0) return null;
  return <SSHSessionsTable sessions={sessions} />;
}

function SSHSessionsTable({ sessions }: { sessions: SSHSession[] }) {
  const { t } = useTranslation();
  return (
    <Table data-testid="ssh-sessions-table">
      <TableHeader>
        <TableRow>
          <TableHead>{t("executors:task")}</TableHead>
          <TableHead>{t("executors:session")}</TableHead>
          <TableHead>{t("executors:host")}</TableHead>
          <TableHead>{t("executors:sshRemotePort")}</TableHead>
          <TableHead>{t("executors:sshLocalForward")}</TableHead>
          <TableHead>{t("executors:uptime")}</TableHead>
          <TableHead>{t("executors:status")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {sessions.map((s) => (
          <SSHSessionsRow key={s.session_id} session={s} />
        ))}
      </TableBody>
    </Table>
  );
}

function SSHSessionsRow({ session: s }: { session: SSHSession }) {
  return (
    <TableRow data-testid={`ssh-session-row-${s.session_id}`}>
      <TableCell className="font-mono text-xs" data-testid="ssh-session-task">
        {s.task_id.slice(0, 8)}
      </TableCell>
      <TableCell className="font-mono text-xs" data-testid="ssh-session-id">
        {s.session_id.slice(0, 8)}
      </TableCell>
      <TableCell className="font-mono text-xs" data-testid="ssh-session-host">
        {s.user ? `${s.user}@${s.host}` : s.host}
      </TableCell>
      <TableCell className="font-mono text-xs" data-testid="ssh-session-remote-port">
        {s.remote_agentctl_port ?? "-"}
      </TableCell>
      <TableCell className="font-mono text-xs" data-testid="ssh-session-local-port">
        {s.local_forward_port ?? "-"}
      </TableCell>
      <TableCell className="text-xs" data-testid="ssh-session-uptime">
        {formatUptime(s.uptime_seconds)}
      </TableCell>
      <TableCell>
        <Badge
          data-testid="ssh-session-status"
          data-status={s.status}
          variant={s.status === "running" ? "default" : "secondary"}
        >
          {s.status}
        </Badge>
      </TableCell>
    </TableRow>
  );
}

function formatUptime(s: number): string {
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}
