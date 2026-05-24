"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconCheck, IconCopy, IconLoader2, IconX } from "@tabler/icons-react";
import { toast } from "sonner";
import { probeSSHAgents, probeSSHShells } from "@/lib/api/domains/ssh-api";
import type { SSHAgentReadinessRow } from "@/lib/types/http-ssh";

export interface SSHAgentReadinessCardProps {
  executorId: string;
  /** The shell currently saved on the profile. Drives initial selection + the
   *  shell sent to the probe endpoint. */
  shell?: string;
  /** Persist a shell change up to the profile. Called when the user picks a
   *  different option in the dropdown so the choice survives the page reload
   *  and flows into the next agent launch as ssh_shell metadata. */
  onShellChange?: (shell: string) => void | Promise<void>;
}

const DEFAULT_SHELL = "bash";

// useReadinessState owns the card's fetch + selection state so the component
// renders a thin view layer. Pulled out to keep the component body under the
// max-lines-per-function lint budget.
function useReadinessState({
  executorId,
  shellProp,
  onShellChange,
}: {
  executorId: string;
  shellProp?: string;
  onShellChange?: (s: string) => void | Promise<void>;
}) {
  const [rows, setRows] = useState<SSHAgentReadinessRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasProbed, setHasProbed] = useState(false);
  const [shells, setShells] = useState<string[] | null>(null);
  const [shellsLoading, setShellsLoading] = useState(false);
  const [shell, setShell] = useState(shellProp ?? DEFAULT_SHELL);
  // Drop stale responses if the user clicks Refresh again before the
  // previous request lands — same pattern as ssh-sessions-card.
  const seqRef = useRef(0);

  const refresh = useCallback(async () => {
    const seq = ++seqRef.current;
    setLoading(true);
    setError(null);
    try {
      const resp = await probeSSHAgents(executorId, { shell });
      if (seq !== seqRef.current) return;
      setRows(resp.rows);
      setHasProbed(true);
    } catch (e) {
      if (seq !== seqRef.current) return;
      setError(e instanceof Error ? e.message : "Failed to probe agents");
    } finally {
      if (seq === seqRef.current) setLoading(false);
    }
  }, [executorId, shell]);

  // Auto-probe the shells once on mount so the dropdown has real options.
  // Cheap (one SSH dial, ~1s) and fully out-of-band of agent probing.
  const probeShells = useCallback(async () => {
    setShellsLoading(true);
    try {
      const resp = await probeSSHShells(executorId);
      setShells(resp.available);
    } catch {
      // On failure the dropdown stays empty; user can type / save the shell
      // via the profile form. We don't toast — a transient probe error
      // shouldn't nag.
    } finally {
      setShellsLoading(false);
    }
  }, [executorId]);

  useEffect(() => {
    // Switching to a different executor should reset the displayed
    // probe state — otherwise the previous executor's rows / error /
    // hasProbed sit in the UI until the next manual refresh, which
    // reads like "still loading" while actually showing stale data.
    seqRef.current = 0;
    setRows([]);
    setError(null);
    setHasProbed(false);
    setLoading(false);
    void probeShells();
    return () => {
      seqRef.current = -1;
    };
  }, [executorId, probeShells]);

  const handleShellChange = useCallback(
    async (next: string) => {
      setShell(next);
      // Stale rows = stale answer (PATH depends on shell init). Clear the
      // table so the user re-probes explicitly with the new shell.
      setRows([]);
      setHasProbed(false);
      if (onShellChange) {
        try {
          await onShellChange(next);
        } catch (e) {
          toast.error(e instanceof Error ? e.message : "Failed to save shell");
        }
      }
    },
    [onShellChange],
  );

  return {
    rows,
    loading,
    error,
    hasProbed,
    shells,
    shellsLoading,
    shell,
    refresh,
    handleShellChange,
  };
}

/**
 * Probes the remote host for each kandev-enabled agent's required binary
 * (the first token of the agent's BuildCommand) under the user-chosen login
 * shell. Surfaces availability + a copy-button for the agent's install
 * command — installs themselves stay manual so the user keeps full control
 * over what runs on their machine. Manual refresh only — a real SSH dial
 * happens per probe so we don't poll on a timer.
 */
export function SSHAgentReadinessCard({
  executorId,
  shell: shellProp,
  onShellChange,
}: SSHAgentReadinessCardProps) {
  const state = useReadinessState({ executorId, shellProp, onShellChange });
  const {
    rows,
    loading,
    error,
    hasProbed,
    shells,
    shellsLoading,
    shell,
    refresh,
    handleShellChange,
  } = state;

  return (
    <Card data-testid="ssh-agent-readiness-card">
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>Available agents on this host</CardTitle>
            <CardDescription>
              Probes the remote {"$PATH"} for each enabled agent under the chosen login shell. Copy
              the install hint and run it on the remote when an agent is missing.
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={refresh}
            disabled={loading}
            data-testid="ssh-agent-readiness-probe"
            className="cursor-pointer shrink-0"
          >
            {loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            {hasProbed ? "Re-probe" : "Probe agents"}
          </Button>
        </div>
        <ShellSelector
          shell={shell}
          shells={shells}
          loading={shellsLoading}
          onChange={handleShellChange}
        />
      </CardHeader>
      <CardContent>
        <ReadinessContent error={error} hasProbed={hasProbed} rows={rows} />
      </CardContent>
    </Card>
  );
}

function ShellSelector({
  shell,
  shells,
  loading,
  onChange,
}: {
  shell: string;
  shells: string[] | null;
  loading: boolean;
  onChange: (s: string) => void | Promise<void>;
}) {
  // While probing, show a placeholder option so the dropdown can't surface
  // a stale "bash" selection that we haven't confirmed exists on the host.
  // After probing, render whatever the host has, plus the current value so
  // a saved shell that isn't actually installed is still selectable + loud.
  const options = uniqueShellOptions(shell, shells);
  return (
    <div className="flex items-center gap-2 pt-3">
      <Label htmlFor="ssh-readiness-shell" className="text-xs text-muted-foreground">
        Login shell
      </Label>
      <Select value={shell} onValueChange={(v) => void onChange(v)} disabled={loading}>
        <SelectTrigger
          id="ssh-readiness-shell"
          data-testid="ssh-readiness-shell"
          className="h-7 w-32 text-xs"
        >
          <SelectValue placeholder="bash" />
        </SelectTrigger>
        <SelectContent>
          {options.map((s) => (
            <SelectItem key={s} value={s} className="text-xs">
              {s}
              {shells && !shells.includes(s) ? (
                <span className="text-amber-600 ml-1">(not detected)</span>
              ) : null}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {loading ? <IconLoader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" /> : null}
    </div>
  );
}

function uniqueShellOptions(currentShell: string, detected: string[] | null): string[] {
  const set = new Set<string>();
  if (detected) detected.forEach((s) => set.add(s));
  set.add(currentShell || DEFAULT_SHELL);
  if (set.size === 0) set.add(DEFAULT_SHELL);
  return Array.from(set);
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
