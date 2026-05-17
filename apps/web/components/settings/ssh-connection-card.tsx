"use client";

import { useCallback, useMemo, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  IconCheck,
  IconLoader2,
  IconShieldLock,
  IconTerminal2,
  IconTestPipe,
  IconX,
} from "@tabler/icons-react";
import { testSSHConnection } from "@/lib/api/domains/ssh-api";
import type {
  SSHIdentitySource,
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
  // Existing running sessions for this executor. Triggers the
  // "this won't affect existing sessions" warning on save.
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

// Fields whose value, if changed, could route the next connection to a
// different machine — editing any of them invalidates the current trust tick
// so the user must re-test against the new target.
const CONNECTION_FIELDS = new Set<keyof SSHExecutorConfig>([
  "host_alias",
  "host",
  "port",
  "user",
  "identity_source",
  "identity_file",
  "proxy_jump",
]);

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
      setState((prev) => {
        const isConnectionField = CONNECTION_FIELDS.has(key);
        return {
          ...prev,
          form: { ...prev.form, [key]: value },
          // Editing a connection-affecting field invalidates the trust gate
          // but keeps the prior result visible so the user can re-tick after
          // a quick re-test; editing the display-only `name` leaves trust
          // alone.
          trust: isConnectionField ? false : prev.trust,
          error: null,
        };
      });
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
    <Card data-testid="ssh-connection-card">
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
        {c.error && (
          <p data-testid="ssh-error" className="text-sm text-red-600">
            {c.error}
          </p>
        )}
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

type FieldOnChange = <K extends keyof SSHExecutorConfig>(
  key: K,
  value: SSHExecutorConfig[K],
) => void;

function SSHConnectionForm({
  form,
  onChange,
}: {
  form: SSHExecutorConfig;
  onChange: FieldOnChange;
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <TextField
        id="ssh-name"
        testId="ssh-input-name"
        label="Name"
        placeholder="My VPS"
        value={form.name}
        onChange={(v) => onChange("name", v)}
      />
      <TextField
        id="ssh-host-alias"
        testId="ssh-input-host-alias"
        label="Host alias from ~/.ssh/config (optional)"
        hint="If set, inherits HostName / Port / User / IdentityFile / ProxyJump from your config."
        placeholder="prod"
        value={form.host_alias ?? ""}
        onChange={(v) => onChange("host_alias", v)}
      />
      <TextField
        id="ssh-host"
        testId="ssh-input-host"
        label="Host"
        placeholder="dev.example.com"
        value={form.host ?? ""}
        onChange={(v) => onChange("host", v)}
      />
      <TextField
        id="ssh-port"
        testId="ssh-input-port"
        label="Port"
        type="number"
        placeholder="22"
        value={String(form.port ?? 22)}
        onChange={(v) => onChange("port", parseInt(v, 10) || 22)}
      />
      <TextField
        id="ssh-user"
        testId="ssh-input-user"
        label="User"
        placeholder="ubuntu"
        value={form.user ?? ""}
        onChange={(v) => onChange("user", v)}
      />
      <IdentitySourceField
        value={form.identity_source}
        onChange={(v) => onChange("identity_source", v)}
      />
      {form.identity_source === "file" && (
        <TextField
          id="ssh-identity-file"
          testId="ssh-input-identity-file"
          label="Identity file path"
          hint="Passphrase-protected keys must be loaded into ssh-agent first."
          placeholder="~/.ssh/id_ed25519"
          value={form.identity_file ?? ""}
          onChange={(v) => onChange("identity_file", v)}
        />
      )}
      <TextField
        id="ssh-proxy-jump"
        testId="ssh-input-proxy-jump"
        label="ProxyJump (optional)"
        hint="Single bastion hop. Chained jumps unsupported in v1."
        placeholder="bastion.example.com"
        value={form.proxy_jump ?? ""}
        onChange={(v) => onChange("proxy_jump", v)}
      />
    </div>
  );
}

function TextField({
  id,
  testId,
  label,
  hint,
  placeholder,
  type,
  value,
  onChange,
}: {
  id: string;
  testId: string;
  label: string;
  hint?: string;
  placeholder?: string;
  type?: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <FieldShell id={id} label={label} hint={hint}>
      <Input
        id={id}
        data-testid={testId}
        type={type}
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
      />
    </FieldShell>
  );
}

function IdentitySourceField({
  value,
  onChange,
}: {
  value: SSHIdentitySource;
  onChange: (v: SSHIdentitySource) => void;
}) {
  return (
    <FieldShell id="ssh-identity-source" label="Identity source">
      <Select value={value} onValueChange={(v) => onChange(v as SSHIdentitySource)}>
        <SelectTrigger id="ssh-identity-source" data-testid="ssh-input-identity-source">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="agent" data-testid="ssh-input-identity-source-agent">
            ssh-agent (SSH_AUTH_SOCK)
          </SelectItem>
          <SelectItem value="file" data-testid="ssh-input-identity-source-file">
            Identity file (private key path)
          </SelectItem>
        </SelectContent>
      </Select>
    </FieldShell>
  );
}

function FieldShell({
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

function PinnedFingerprintRow({ fingerprint }: { fingerprint: string }) {
  return (
    <div
      data-testid="ssh-fingerprint-pinned"
      className="rounded-md border bg-muted/40 px-3 py-2 text-xs flex items-center gap-2"
    >
      <IconShieldLock className="h-4 w-4 shrink-0" />
      <span className="text-muted-foreground">
        Pinned fingerprint:{" "}
        <code data-testid="ssh-fingerprint-pinned-value" className="font-mono">
          {fingerprint}
        </code>
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
        data-testid="ssh-test-button"
        className="cursor-pointer"
      >
        {testing ? (
          <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" />
        ) : (
          <IconTestPipe className="mr-1.5 h-4 w-4" />
        )}
        Test connection
      </Button>
      <Button
        size="sm"
        onClick={onSave}
        disabled={!canSave}
        data-testid="ssh-save-button"
        className="cursor-pointer"
      >
        {saving ? <IconLoader2 className="mr-1.5 h-4 w-4 animate-spin" /> : null}
        Save
      </Button>
    </div>
  );
}

function ConnectionBadge({ fingerprint }: { fingerprint?: string }) {
  if (!fingerprint) {
    return (
      <Badge data-testid="ssh-connection-badge" data-status="unverified" variant="secondary">
        Unverified
      </Badge>
    );
  }
  return (
    <Badge
      data-testid="ssh-connection-badge"
      data-status="trusted"
      variant="default"
      className="bg-green-600"
    >
      Trusted
    </Badge>
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
  return (
    <div
      data-testid="ssh-test-result"
      data-success={result.success ? "true" : "false"}
      className="rounded-md border p-3 space-y-2"
    >
      <TestResultHeader success={result.success} totalMs={result.total_duration_ms} />
      {result.steps.map((step: SSHTestStep) => (
        <StepRow key={step.name} step={step} />
      ))}
      {result.error && !result.steps.some((s) => s.error) && (
        <p data-testid="ssh-test-result-error" className="text-sm text-red-600">
          {result.error}
        </p>
      )}
      {result.success && result.fingerprint && (
        <FingerprintTrustBlock
          fingerprint={result.fingerprint}
          currentlyPinned={currentlyPinned}
          trust={trust}
          onTrustChange={onTrustChange}
        />
      )}
    </div>
  );
}

function TestResultHeader({ success, totalMs }: { success: boolean; totalMs: number }) {
  return (
    <div
      data-testid={success ? "ssh-test-result-success" : "ssh-test-result-failure"}
      className="flex items-center gap-2 text-sm font-medium"
    >
      {success ? (
        <IconCheck className="h-4 w-4 text-green-600" />
      ) : (
        <IconX className="h-4 w-4 text-red-600" />
      )}
      {success ? "Connection test passed" : "Connection test failed"}
      <span className="text-muted-foreground font-normal">({totalMs}ms)</span>
    </div>
  );
}

function FingerprintTrustBlock({
  fingerprint,
  currentlyPinned,
  trust,
  onTrustChange,
}: {
  fingerprint: string;
  currentlyPinned?: string;
  trust: boolean;
  onTrustChange: (v: boolean) => void;
}) {
  const fingerprintChanged = !!currentlyPinned && currentlyPinned !== fingerprint;
  return (
    <div className="mt-3 space-y-2">
      <div className="text-xs">
        <span className="text-muted-foreground">Host fingerprint observed: </span>
        <code data-testid="ssh-fingerprint-observed" className="font-mono">
          {fingerprint}
        </code>
      </div>
      {fingerprintChanged && (
        <p data-testid="ssh-fingerprint-change-warning" className="text-xs text-amber-600">
          Warning: this fingerprint differs from the one currently pinned (
          <code className="font-mono">{currentlyPinned}</code>). Trusting it overwrites the pinned
          key. If you didn’t expect a host re-key, stop here.
        </p>
      )}
      <label className="flex items-start gap-2 text-sm cursor-pointer">
        <input
          type="checkbox"
          data-testid="ssh-trust-checkbox"
          checked={trust}
          onChange={(e) => onTrustChange(e.target.checked)}
          className="mt-0.5 cursor-pointer"
        />
        <span>
          <strong>Trust this host.</strong> I’ve verified the fingerprint above matches my server.
          Future connections that report a different fingerprint will be refused.
        </span>
      </label>
    </div>
  );
}

function StepRow({ step }: { step: SSHTestStep }) {
  const slug = step.name.toLowerCase().replace(/[^a-z0-9]+/g, "-");
  return (
    <div
      data-testid={`ssh-test-step-${slug}`}
      data-success={step.success ? "true" : "false"}
      className="flex items-start gap-2 text-sm pl-2"
    >
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
          <p
            data-testid={`ssh-test-step-${slug}-output`}
            className="text-xs text-muted-foreground truncate font-mono"
          >
            {step.output}
          </p>
        )}
        {step.error && (
          <p data-testid={`ssh-test-step-${slug}-error`} className="text-xs text-red-600 truncate">
            {step.error}
          </p>
        )}
      </div>
    </div>
  );
}
