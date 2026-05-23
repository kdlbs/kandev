"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconCheck, IconCopy, IconLoader2, IconX } from "@tabler/icons-react";
import { toast } from "sonner";
import { probeSSHAgents } from "@/lib/api/domains/ssh-api";
import type { SSHAgentReadinessRow } from "@/lib/types/http-ssh";

export interface SSHAgentReadinessCardProps {
  executorId: string;
}

/**
 * Probes the remote host for each kandev-enabled agent's required binary
 * (the first token of the agent's BuildCommand). Surfaces availability + a
 * copy-button for the agent's install command. Manual refresh only — a real
 * SSH dial happens per probe so we don't poll on a timer.
 */
export function SSHAgentReadinessCard({ executorId }: SSHAgentReadinessCardProps) {
  const [rows, setRows] = useState<SSHAgentReadinessRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasProbed, setHasProbed] = useState(false);
  // Drop stale responses if the user clicks Refresh again before the
  // previous request lands — same pattern as ssh-sessions-card.
  const seqRef = useRef(0);

  const refresh = useCallback(async () => {
    const seq = ++seqRef.current;
    setLoading(true);
    setError(null);
    try {
      const resp = await probeSSHAgents(executorId);
      if (seq !== seqRef.current) return;
      setRows(resp.rows);
      setHasProbed(true);
    } catch (e) {
      if (seq !== seqRef.current) return;
      setError(e instanceof Error ? e.message : "Failed to probe agents");
    } finally {
      if (seq === seqRef.current) setLoading(false);
    }
  }, [executorId]);

  useEffect(() => {
    seqRef.current = 0;
    return () => {
      seqRef.current = -1;
    };
  }, [executorId]);

  return (
    <Card data-testid="ssh-agent-readiness-card">
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Available agents on this host</CardTitle>
            <CardDescription>
              Probes the remote {"$PATH"} for each enabled agent. Use the install hint to add a
              missing agent on the remote.
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={refresh}
            disabled={loading}
            data-testid="ssh-agent-readiness-probe"
            className="cursor-pointer"
          >
            {loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {hasProbed ? "Re-probe" : "Probe agents"}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <ReadinessContent error={error} hasProbed={hasProbed} rows={rows} />
      </CardContent>
    </Card>
  );
}

function ReadinessContent({
  error,
  hasProbed,
  rows,
}: {
  error: string | null;
  hasProbed: boolean;
  rows: SSHAgentReadinessRow[];
}) {
  if (error) {
    return (
      <p data-testid="ssh-agent-readiness-error" className="text-sm text-red-600 dark:text-red-400">
        {error}
      </p>
    );
  }
  if (!hasProbed) {
    return (
      <p className="text-sm text-muted-foreground">
        Click {`"Probe agents"`} to check which agents are installed on the remote.
      </p>
    );
  }
  if (rows.length === 0) {
    return <p className="text-sm text-muted-foreground">No enabled agents to probe.</p>;
  }
  return <ReadinessTable rows={rows} />;
}

function ReadinessTable({ rows }: { rows: SSHAgentReadinessRow[] }) {
  return (
    <Table data-testid="ssh-agent-readiness-table">
      <TableHeader>
        <TableRow>
          <TableHead>Agent</TableHead>
          <TableHead>Binary</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Install hint</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <ReadinessRow key={row.agent_id} row={row} />
        ))}
      </TableBody>
    </Table>
  );
}

function ReadinessRow({ row }: { row: SSHAgentReadinessRow }) {
  const slug = row.agent_id.replace(/[^a-z0-9]+/gi, "-");
  return (
    <TableRow data-testid={`ssh-readiness-row-${slug}`} data-available={row.available}>
      <TableCell className="font-medium">{row.agent_name || row.agent_id}</TableCell>
      <TableCell className="font-mono text-xs">{row.binary}</TableCell>
      <TableCell>
        <StatusBadge row={row} />
      </TableCell>
      <TableCell className="text-xs">
        <InstallHint hint={row.install_hint} available={row.available} />
      </TableCell>
    </TableRow>
  );
}

function StatusBadge({ row }: { row: SSHAgentReadinessRow }) {
  if (row.error) {
    return (
      <Badge variant="outline" className="border-amber-500/30 bg-amber-500/10 text-amber-700">
        <IconX className="mr-1 h-3 w-3" /> Probe error
      </Badge>
    );
  }
  if (row.available) {
    return (
      <Badge variant="outline" className="border-green-500/30 bg-green-500/10 text-green-700">
        <IconCheck className="mr-1 h-3 w-3" /> Installed
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="border-red-500/30 bg-red-500/10 text-red-700">
      <IconX className="mr-1 h-3 w-3" /> Missing
    </Badge>
  );
}

function InstallHint({ hint, available }: { hint?: string; available: boolean }) {
  if (available) return <span className="text-muted-foreground">—</span>;
  if (!hint) return <span className="text-muted-foreground">No hint available</span>;
  return (
    <div className="flex items-center gap-1">
      <code className="truncate">{hint}</code>
      <button
        type="button"
        className="cursor-pointer text-muted-foreground hover:text-foreground"
        aria-label="Copy install hint"
        onClick={() => {
          void navigator.clipboard.writeText(hint).then(() => toast.success("Install hint copied"));
        }}
      >
        <IconCopy className="h-3 w-3" />
      </button>
    </div>
  );
}
