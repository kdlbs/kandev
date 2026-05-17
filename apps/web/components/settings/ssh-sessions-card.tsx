"use client";

import { useCallback, useEffect, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconLoader2 } from "@tabler/icons-react";
import { listSSHSessions } from "@/lib/api/domains/ssh-api";
import type { SSHSession } from "@/lib/types/http-ssh";

export interface SSHSessionsCardProps {
  executorId: string;
}

const REFRESH_INTERVAL_MS = 90_000;

export function SSHSessionsCard({ executorId }: SSHSessionsCardProps) {
  const [sessions, setSessions] = useState<SSHSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const rows = await listSSHSessions(executorId);
      setSessions(rows);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  }, [executorId]);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, REFRESH_INTERVAL_MS);
    return () => clearInterval(id);
  }, [refresh]);

  return (
    <Card data-testid="ssh-sessions-card">
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Active sessions</CardTitle>
            <CardDescription>
              Sessions currently running on this SSH host. Refreshes every 90 seconds.
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={refresh}
            disabled={loading}
            data-testid="ssh-sessions-refresh"
            className="cursor-pointer"
          >
            {loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            Refresh
          </Button>
        </div>
      </CardHeader>
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
        No active sessions.
      </p>
    );
  }
  if (sessions.length === 0) return null;
  return <SSHSessionsTable sessions={sessions} />;
}

function SSHSessionsTable({ sessions }: { sessions: SSHSession[] }) {
  return (
    <Table data-testid="ssh-sessions-table">
      <TableHeader>
        <TableRow>
          <TableHead>Task</TableHead>
          <TableHead>Session</TableHead>
          <TableHead>Host</TableHead>
          <TableHead>Remote port</TableHead>
          <TableHead>Local fwd</TableHead>
          <TableHead>Uptime</TableHead>
          <TableHead>Status</TableHead>
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
        {s.remote_agentctl_port ?? "—"}
      </TableCell>
      <TableCell className="font-mono text-xs" data-testid="ssh-session-local-port">
        {s.local_forward_port ?? "—"}
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
