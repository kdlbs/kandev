"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import {
  IconCheck,
  IconLoader2,
  IconShieldLock,
  IconTerminal2,
  IconTestPipe,
  IconX,
} from "@tabler/icons-react";
import { testSSHConnection, listSSHSessions } from "@/lib/api/domains/ssh-api";
import type {
  SSHIdentitySource,
  SSHSession,
  SSHTestRequest,
  SSHTestResult,
  SSHTestStep,
} from "@/lib/types/http-ssh";

// SSHExecutorConfig is the shape we persist into executor.Config on save.
// The host_fingerprint is set only after a successful Test Connection has
// completed and the user has ticked "Trust this host".
export interface SSHExecutorConfig {
  name: string;
  host_alias?: string;
  host?: string;
  port?: number;
  user?: string;
  identity_source: SSHIdentitySource;
  identity_file?: string;
  proxy_jump?: string;
  host_fingerprint?: string;
}

export interface SSHConnectionCardProps {
  initial?: Partial<SSHExecutorConfig>;
  // Called when the user clicks Save after a successful test+trust. The
  // returned config carries the freshly pinned fingerprint.
  onSave: (config: SSHExecutorConfig) => Promise<void> | void;
  // Existing running sessions for this executor. Triggers the "this won't
  // affect existing sessions" warning on save.
  runningSessionCount?: number;
}

interface SSHConnectionState {
  form: SSHExecutorConfig;
  testing: boolean;
  saving: boolean;
  result: SSHTestResult | null;
  trust: boolean;
  error: string | null;
}

const SSH_FORM_DEFAULTS: SSHExecutorConfig = {
  name: "",
  host_alias: "",
  host: "",
  port: 22,
  user: "",
  identity_source: "agent",
  identity_file: "",
  proxy_jump: "",
  host_fingerprint: undefined,
};

function initialState(initial?: Partial<SSHExecutorConfig>): SSHConnectionState {
  return {
    form: { ...SSH_FORM_DEFAULTS, ...(initial ?? {}) },
    testing: false,
    saving: false,
    result: null,
    trust: false,
    error: null,
  };
}

function useSSHConnection(props: SSHConnectionCardProps) {
  const [state, setState] = useState<SSHConnectionState>(() => initialState(props.initial));
  const { form, testing, saving, result, trust, error } = state;

  const update = useCallback(
    <K extends keyof SSHExecutorConfig>(key: K, value: SSHExecutorConfig[K]) => {
      setState((prev) => ({
        ...prev,
        form: { ...prev.form, [key]: value },
        result: null,
        trust: false,
        error: null,
      }));
    },
    [],
  );

  const setTrust = useCallback((v: boolean) => setState((prev) => ({ ...prev, trust: v })), []);

  const canTest = useMemo(() => {
    if (testing) return false;
    if (form.name.trim() === "") return false;
    if ((form.host ?? "").trim() === "" && (form.host_alias ?? "").trim() === "") return false;
    return true;
  }, [form, testing]);

  const handleTest = useCallback(async () => {
    setState((prev) => ({ ...prev, testing: true, result: null, error: null }));
    try {
      const req: SSHTestRequest = {
        name: form.name,
        host_alias: form.host_alias || undefined,
        host: form.host || undefined,
        port: form.port || undefined,
        user: form.user || undefined,
        identity_source: form.identity_source,
        identity_file: form.identity_file || undefined,
        proxy_jump: form.proxy_jump || undefined,
      };
      const res = await testSSHConnection(req);
      setState((prev) => ({ ...prev, result: res, testing: false }));
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Failed to reach backend";
      setState((prev) => ({ ...prev, error: msg, testing: false }));
    }
  }, [form]);

  const canSave = !!result?.success && !!result.fingerprint && trust && !saving;

  const confirmRunningSessions = useCallback(
    () =>
      props.runningSessionCount
        ? window.confirm(
            `This executor has ${props.runningSessionCount} running session(s). ` +
              `They will keep running on the current host. Only new sessions started ` +
              `after save will use the updated config. Continue?`,
          )
        : true,
    [props.runningSessionCount],
  );

  const handleSave = useCallback(async () => {
    if (!canSave || !result?.fingerprint) return;
    if (!confirmRunningSessions()) return;
    setState((prev) => ({ ...prev, saving: true }));
    try {
      await props.onSave({ ...form, host_fingerprint: result.fingerprint });
    } finally {
      setState((prev) => ({ ...prev, saving: false }));
    }
  }, [canSave, confirmRunningSessions, form, props, result]);

  return {
    form,
    testing,
    saving,
    result,
    trust,
    error,
    canTest,
    canSave,
    update,
    setTrust,
    handleTest,
    handleSave,
  };
}

export function SSHConnectionCard(props: SSHConnectionCardProps) {
  const c = useSSHConnection(props);
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="flex items-center gap-2">
              <IconTerminal2 className="h-5 w-5" />
              Connection
            </CardTitle>
            <CardDescription>
              Run an agent on any Linux box you can reach over SSH. linux/amd64 only in v1.
            </CardDescription>
          </div>
          <ConnectionBadge fingerprint={c.form.host_fingerprint} />
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        <SSHConnectionForm form={c.form} onChange={c.update} />
        {c.form.host_fingerprint && <PinnedFingerprintRow fingerprint={c.form.host_fingerprint} />}
        <SSHConnectionActions
          testing={c.testing}
          saving={c.saving}
          canTest={c.canTest}
          canSave={c.canSave}
          onTest={c.handleTest}
          onSave={c.handleSave}
        />
        {c.error && <p className="text-sm text-red-600">{c.error}</p>}
        {c.result && (
          <TestResultDisplay
            result={c.result}
            trust={c.trust}
            onTrustChange={c.setTrust}
            currentlyPinned={c.form.host_fingerprint}
          />
        )}
      </CardContent>
    </Card>
  );
}

function SSHConnectionForm({
  form,
  onChange,
}: {
  form: SSHExecutorConfig;
  onChange: <K extends keyof SSHExecutorConfig>(key: K, value: SSHExecutorConfig[K]) => void;
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <Field id="ssh-name" label="Name">
        <Input
          id="ssh-name"
          value={form.name}
          onChange={(e) => onChange("name", e.target.value)}
          placeholder="My VPS"
        />
      </Field>
      <Field
        id="ssh-host-alias"
        label="Host alias from ~/.ssh/config (optional)"
        hint="If set, inherits HostName / Port / User / IdentityFile / ProxyJump from your config."
      >
        <Input
          id="ssh-host-alias"
          value={form.host_alias ?? ""}
          onChange={(e) => onChange("host_alias", e.target.value)}
          placeholder="prod"
        />
      </Field>
      <Field id="ssh-host" label="Host">
        <Input
          id="ssh-host"
          value={form.host ?? ""}
          onChange={(e) => onChange("host", e.target.value)}
          placeholder="dev.example.com"
        />
      </Field>
      <Field id="ssh-port" label="Port">
        <Input
          id="ssh-port"
          type="number"
          value={form.port ?? 22}
          onChange={(e) => onChange("port", parseInt(e.target.value, 10) || 22)}
          placeholder="22"
        />
      </Field>
      <Field id="ssh-user" label="User">
        <Input
          id="ssh-user"
          value={form.user ?? ""}
          onChange={(e) => onChange("user", e.target.value)}
          placeholder="ubuntu"
        />
      </Field>
      <Field id="ssh-identity-source" label="Identity source">
        <Select
          value={form.identity_source}
          onValueChange={(v) => onChange("identity_source", v as SSHIdentitySource)}
        >
          <SelectTrigger id="ssh-identity-source">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="agent">ssh-agent (SSH_AUTH_SOCK)</SelectItem>
            <SelectItem value="file">Identity file (private key path)</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      {form.identity_source === "file" && (
        <Field
          id="ssh-identity-file"
          label="Identity file path"
          hint="Passphrase-protected keys must be loaded into ssh-agent first."
        >
          <Input
            id="ssh-identity-file"
            value={form.identity_file ?? ""}
            onChange={(e) => onChange("identity_file", e.target.value)}
            placeholder="~/.ssh/id_ed25519"
          />
        </Field>
      )}
      <Field
        id="ssh-proxy-jump"
        label="ProxyJump (optional)"
        hint="Single bastion hop. Chained jumps unsupported in v1."
      >
        <Input
          id="ssh-proxy-jump"
          value={form.proxy_jump ?? ""}
          onChange={(e) => onChange("proxy_jump", e.target.value)}
          placeholder="bastion.example.com"
        />
      </Field>
    </div>
  );
}

function PinnedFingerprintRow({ fingerprint }: { fingerprint: string }) {
  return (
    <div className="rounded-md border bg-muted/40 px-3 py-2 text-xs flex items-center gap-2">
      <IconShieldLock className="h-4 w-4 shrink-0" />
      <span className="text-muted-foreground">
        Pinned fingerprint: <code className="font-mono">{fingerprint}</code>
      </span>
    </div>
  );
}

function SSHConnectionActions({
  testing,
  saving,
  canTest,
  canSave,
  onTest,
  onSave,
}: {
  testing: boolean;
  saving: boolean;
  canTest: boolean;
  canSave: boolean;
  onTest: () => void;
  onSave: () => void;
}) {
  return (
    <div className="flex items-center gap-3">
      <Button
        variant="outline"
        size="sm"
        onClick={onTest}
        disabled={!canTest}
        className="cursor-pointer"
      >
        {testing ? (
          <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
        ) : (
          <IconTestPipe className="mr-1.5 h-4 w-4" />
        )}
        Test connection
      </Button>
      <Button size="sm" onClick={onSave} disabled={!canSave} className="cursor-pointer">
        {saving ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
        Save
      </Button>
    </div>
  );
}

function ConnectionBadge({ fingerprint }: { fingerprint?: string }) {
  if (!fingerprint) return <Badge variant="secondary">Unverified</Badge>;
  return (
    <Badge variant="default" className="bg-green-600">
      Trusted
    </Badge>
  );
}

function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function TestResultDisplay({
  result,
  trust,
  onTrustChange,
  currentlyPinned,
}: {
  result: SSHTestResult;
  trust: boolean;
  onTrustChange: (v: boolean) => void;
  currentlyPinned?: string;
}) {
  const fingerprintChanged =
    !!currentlyPinned && !!result.fingerprint && currentlyPinned !== result.fingerprint;
  return (
    <div className="rounded-md border p-3 space-y-2">
      <div className="flex items-center gap-2 text-sm font-medium">
        {result.success ? (
          <IconCheck className="h-4 w-4 text-green-600" />
        ) : (
          <IconX className="h-4 w-4 text-red-600" />
        )}
        {result.success ? "Connection test passed" : "Connection test failed"}
        <span className="text-muted-foreground font-normal">({result.total_duration_ms}ms)</span>
      </div>
      {result.steps.map((step: SSHTestStep) => (
        <StepRow key={step.name} step={step} />
      ))}
      {result.error && !result.steps.some((s) => s.error) && (
        <p className="text-sm text-red-600">{result.error}</p>
      )}
      {result.success && result.fingerprint && (
        <div className="mt-3 space-y-2">
          <div className="text-xs">
            <span className="text-muted-foreground">Host fingerprint observed: </span>
            <code className="font-mono">{result.fingerprint}</code>
          </div>
          {fingerprintChanged && (
            <p className="text-xs text-amber-600">
              Warning: this fingerprint differs from the one currently pinned (
              <code className="font-mono">{currentlyPinned}</code>). Trusting it overwrites the
              pinned key. If you didn’t expect a host re-key, stop here.
            </p>
          )}
          <label className="flex items-start gap-2 text-sm cursor-pointer">
            <input
              type="checkbox"
              checked={trust}
              onChange={(e) => onTrustChange(e.target.checked)}
              className="mt-0.5 cursor-pointer"
            />
            <span>
              <strong>Trust this host.</strong> I’ve verified the fingerprint above matches my
              server. Future connections that report a different fingerprint will be refused.
            </span>
          </label>
        </div>
      )}
    </div>
  );
}

function StepRow({ step }: { step: SSHTestStep }) {
  return (
    <div className="flex items-start gap-2 text-sm pl-2">
      {step.success ? (
        <IconCheck className="h-3 w-3 text-green-600 shrink-0 mt-1" />
      ) : (
        <IconX className="h-3 w-3 text-red-600 shrink-0 mt-1" />
      )}
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span>{step.name}</span>
          <span className="text-muted-foreground text-xs">({step.duration_ms}ms)</span>
        </div>
        {step.output && (
          <p className="text-xs text-muted-foreground truncate font-mono">{step.output}</p>
        )}
        {step.error && <p className="text-xs text-red-600 truncate">{step.error}</p>}
      </div>
    </div>
  );
}

export interface SSHSessionsCardProps {
  executorId: string;
}

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
    const id = setInterval(refresh, 90_000);
    return () => clearInterval(id);
  }, [refresh]);

  return (
    <Card>
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
            className="cursor-pointer"
          >
            {loading ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
            Refresh
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {error && <p className="text-sm text-red-600">{error}</p>}
        {!error && sessions.length === 0 && !loading && (
          <p className="text-sm text-muted-foreground">No active sessions.</p>
        )}
        {sessions.length > 0 && (
          <Table>
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
                <TableRow key={s.session_id}>
                  <TableCell className="font-mono text-xs">{s.task_id.slice(0, 8)}</TableCell>
                  <TableCell className="font-mono text-xs">{s.session_id.slice(0, 8)}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {s.user ? `${s.user}@${s.host}` : s.host}
                  </TableCell>
                  <TableCell className="font-mono text-xs">
                    {s.remote_agentctl_port ?? "—"}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{s.local_forward_port ?? "—"}</TableCell>
                  <TableCell className="text-xs">{formatUptime(s.uptime_seconds)}</TableCell>
                  <TableCell>
                    <Badge variant={s.status === "running" ? "default" : "secondary"}>
                      {s.status}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function formatUptime(s: number): string {
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}
