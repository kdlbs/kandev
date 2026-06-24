"use client";

import { useCallback, useEffect, useState } from "react";
import {
  IconBrandSentry,
  IconInfoCircle,
  IconPencil,
  IconPlus,
  IconTrash,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSentryEnabled } from "@/hooks/domains/sentry/use-sentry-enabled";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { ApiError } from "@/lib/api/client";
import {
  listSentryInstances,
  createSentryInstance,
  updateSentryInstance,
  deleteSentryInstance,
  testSentryConnection,
} from "@/lib/api/domains/sentry-api";
import {
  SENTRY_AUTH_METHOD,
  SENTRY_DEFAULT_URL,
  type SentryConfig,
  type TestSentryConnectionResult,
} from "@/lib/types/sentry";
import { SentryIssueWatchersSection } from "./sentry-issue-watchers-section";

type FormState = {
  name: string;
  url: string;
  secret: string;
};

const emptyForm: FormState = { name: "", url: SENTRY_DEFAULT_URL, secret: "" };

function instanceToForm(cfg: SentryConfig): FormState {
  // The secret is write-only: blank means "keep the stored token" on update.
  return { name: cfg.name, url: cfg.url || SENTRY_DEFAULT_URL, secret: "" };
}

function saveLabel(saving: boolean, isEdit: boolean): string {
  if (saving) return "Saving...";
  return isEdit ? "Update" : "Add instance";
}

function configToHealth(config: SentryConfig): IntegrationAuthHealth | null {
  if (!config.hasSecret) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

// describeDeleteError turns the structured 409 IN_USE body into clear guidance,
// falling back to the raw error for everything else.
function describeDeleteError(err: unknown): string {
  if (err instanceof ApiError && err.status === 409) {
    const body = err.body as { code?: string; watchCount?: number } | null;
    if (body?.code === "SENTRY_INSTANCE_IN_USE") {
      const n = body.watchCount ?? 0;
      return `In use by ${n} issue watch${n === 1 ? "" : "es"} — reassign or delete those first`;
    }
  }
  return `Delete failed: ${String(err)}`;
}

type UpdateFn = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

type FieldProps = {
  form: FormState;
  disabled: boolean;
  update: UpdateFn;
};

function NameField({ form, disabled, update }: FieldProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="sentry-name">Name</Label>
      <Input
        id="sentry-name"
        data-testid="sentry-name-input"
        placeholder="Production"
        value={form.name}
        onChange={(e) => update("name", e.target.value)}
        disabled={disabled}
      />
      <p className="text-xs text-muted-foreground">
        A label to tell this instance apart from your others.
      </p>
    </div>
  );
}

function UrlField({ form, disabled, update }: FieldProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="sentry-url">Instance URL</Label>
      <Input
        id="sentry-url"
        data-testid="sentry-url-input"
        type="url"
        placeholder={SENTRY_DEFAULT_URL}
        value={form.url}
        onChange={(e) => update("url", e.target.value)}
        disabled={disabled}
      />
      <p className="text-xs text-muted-foreground">
        Base URL of your Sentry instance. Leave as {SENTRY_DEFAULT_URL} for Sentry SaaS, or point it
        at a self-hosted install (e.g. https://sentry.your-company.com).
      </p>
    </div>
  );
}

function SecretScopesTooltip() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <IconInfoCircle
          className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0"
          aria-label="Required token scopes"
        />
      </TooltipTrigger>
      <TooltipContent className="max-w-xs" align="start">
        <p className="text-xs font-medium mb-1">Grant Read access to these scopes:</p>
        <ul className="text-xs space-y-0.5">
          <li>
            <code className="text-[10px] bg-white/15 px-1 rounded">org:read</code>{" "}
            <span className="opacity-70">Organization — resolve the org and list issues</span>
          </li>
          <li>
            <code className="text-[10px] bg-white/15 px-1 rounded">project:read</code>{" "}
            <span className="opacity-70">Project — list projects and scope searches</span>
          </li>
          <li>
            <code className="text-[10px] bg-white/15 px-1 rounded">event:read</code>{" "}
            <span className="opacity-70">Issue &amp; Event — browse issues and run watchers</span>
          </li>
        </ul>
      </TooltipContent>
    </Tooltip>
  );
}

function SecretField({
  form,
  disabled,
  update,
  hasSavedSecret,
}: FieldProps & { hasSavedSecret: boolean }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label htmlFor="sentry-secret">
          Auth token
          {hasSavedSecret && (
            <span className="text-xs text-muted-foreground ml-2">
              (saved — leave blank to keep the current value)
            </span>
          )}
        </Label>
        <SecretScopesTooltip />
      </div>
      <Input
        id="sentry-secret"
        data-testid="sentry-secret-input"
        type="password"
        placeholder={hasSavedSecret ? "••••••••" : "sntrys_..."}
        value={form.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={disabled}
      />
      <p className="text-xs text-muted-foreground">
        Create a new personal token at{" "}
        <a
          className="underline"
          href="https://sentry.io/settings/account/api/auth-tokens/new-token/"
          target="_blank"
          rel="noreferrer"
        >
          sentry.io → Settings → Auth Tokens
        </a>
      </p>
    </div>
  );
}

function TestResultAlert({ result }: { result: TestSentryConnectionResult | null }) {
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {result.ok
          ? `Connected as ${result.displayName || result.email || result.userId}`
          : `Failed: ${result.error}`}
      </AlertDescription>
    </Alert>
  );
}

type InstanceFormControls = {
  form: FormState;
  testResult: TestSentryConnectionResult | null;
  saving: boolean;
  testing: boolean;
  update: UpdateFn;
  handleTest: () => void;
  handleSave: () => void;
};

// useInstanceForm owns one add/edit form. With an instance it edits (testing the
// saved instance by id, blank secret keeps the stored token); without one it
// creates (testing the unsaved credentials inline before persisting).
function useInstanceForm(instance: SentryConfig | null, onSaved: () => void): InstanceFormControls {
  const { toast } = useToast();
  const [form, setForm] = useState<FormState>(() =>
    instance ? instanceToForm(instance) : emptyForm,
  );
  const [testResult, setTestResult] = useState<TestSentryConnectionResult | null>(null);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  const update = useCallback<UpdateFn>(
    (key, value) => setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testSentryConnection(
        instance
          ? {
              instanceId: instance.id,
              url: form.url || undefined,
              secret: form.secret || undefined,
            }
          : {
              name: form.name || undefined,
              url: form.url || undefined,
              secret: form.secret || undefined,
            },
      );
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [form, instance]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const payload = {
        name: form.name.trim(),
        authMethod: SENTRY_AUTH_METHOD,
        url: form.url,
        secret: form.secret,
      };
      if (instance) {
        await updateSentryInstance(instance.id, payload);
        toast({ description: "Sentry instance updated", variant: "success" });
      } else {
        await createSentryInstance(payload);
        toast({ description: "Sentry instance added", variant: "success" });
      }
      onSaved();
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [form, instance, onSaved, toast]);

  return { form, testResult, saving, testing, update, handleTest, handleSave };
}

type InstanceFormCardProps = {
  instance: SentryConfig | null;
  onSaved: () => void;
  onCancel: () => void;
};

function InstanceFormCard({ instance, onSaved, onCancel }: InstanceFormCardProps) {
  const c = useInstanceForm(instance, onSaved);
  const isEdit = !!instance;
  const hasSavedSecret = !!instance?.hasSecret;
  const busy = c.saving || c.testing;
  const missingName = !c.form.name.trim();
  // Create requires an inline secret; edit may leave it blank to keep the stored
  // token, so only a fresh instance with no secret blocks save/test.
  const missingSecret = !hasSavedSecret && !c.form.secret;
  const disableSave = busy || missingName || missingSecret;
  const disableTest = busy || missingSecret;

  return (
    <div className="space-y-4 rounded-md border bg-muted/20 p-4" data-testid="sentry-instance-form">
      <NameField form={c.form} disabled={busy} update={c.update} />
      <UrlField form={c.form} disabled={busy} update={c.update} />
      <SecretField
        form={c.form}
        disabled={busy}
        update={c.update}
        hasSavedSecret={hasSavedSecret}
      />
      <TestResultAlert result={c.testResult} />
      <Separator />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={c.handleTest}
          disabled={disableTest}
          className="cursor-pointer"
          title={disableTest ? "Paste an auth token to test the connection" : undefined}
          data-testid="sentry-test-button"
        >
          {c.testing ? "Testing..." : "Test connection"}
        </Button>
        <Button
          type="button"
          onClick={c.handleSave}
          disabled={disableSave}
          className="cursor-pointer"
          data-testid="sentry-save-button"
        >
          {saveLabel(c.saving, isEdit)}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={onCancel}
          disabled={c.saving}
          className="ml-auto cursor-pointer"
          data-testid="sentry-cancel-button"
        >
          Cancel
        </Button>
      </div>
    </div>
  );
}

function InstanceCard({
  instance,
  onEdit,
  onDelete,
}: {
  instance: SentryConfig;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const health = configToHealth(instance);
  return (
    <div
      className="space-y-3 rounded-md border p-4"
      data-testid="sentry-instance-card"
      data-instance-name={instance.name}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-medium truncate" data-testid="sentry-instance-name">
            {instance.name}
          </p>
          <p className="text-xs text-muted-foreground truncate">{instance.url}</p>
        </div>
        <div className="flex items-center gap-1 shrink-0">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onEdit}
            className="cursor-pointer gap-1"
            data-testid="sentry-instance-edit"
          >
            <IconPencil className="h-3.5 w-3.5" />
            Edit
          </Button>
          <Button
            type="button"
            size="sm"
            variant="destructive"
            onClick={onDelete}
            className="cursor-pointer gap-1"
            data-testid="sentry-instance-delete"
          >
            <IconTrash className="h-3.5 w-3.5" />
            Delete
          </Button>
        </div>
      </div>
      <IntegrationAuthStatusBanner health={health} />
    </div>
  );
}

function useSentryInstances() {
  const { toast } = useToast();
  const [instances, setInstances] = useState<SentryConfig[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try {
      setInstances(await listSentryInstances());
    } catch (err) {
      toast({ description: `Failed to load Sentry instances: ${String(err)}`, variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const id = setInterval(() => {
      listSentryInstances()
        .then(setInstances)
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, []);

  return { instances, loading, reload: load };
}

function EnabledPill() {
  const { enabled, setEnabled } = useSentryEnabled();
  return (
    <div className="flex items-center gap-2 rounded-full border bg-muted/30 px-3 py-1">
      <Switch
        id="sentry-enabled"
        checked={enabled}
        onCheckedChange={setEnabled}
        className="cursor-pointer"
      />
      <Label htmlFor="sentry-enabled" className="text-xs cursor-pointer">
        {enabled ? "Enabled" : "Disabled"}
      </Label>
    </div>
  );
}

type InstanceRowProps = {
  instance: SentryConfig;
  editing: boolean;
  onEdit: () => void;
  onCancelEdit: () => void;
  onSaved: () => void;
  onDelete: () => void;
};

function InstanceRow({
  instance,
  editing,
  onEdit,
  onCancelEdit,
  onSaved,
  onDelete,
}: InstanceRowProps) {
  if (editing) {
    return <InstanceFormCard instance={instance} onSaved={onSaved} onCancel={onCancelEdit} />;
  }
  return <InstanceCard instance={instance} onEdit={onEdit} onDelete={onDelete} />;
}

function InstancesBody({
  loading,
  instances,
  adding,
  editingId,
  setEditingId,
  onSaved,
  onDelete,
}: {
  loading: boolean;
  instances: SentryConfig[];
  adding: boolean;
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  onSaved: () => void;
  onDelete: (instance: SentryConfig) => void;
}) {
  if (loading) return <p className="text-sm text-muted-foreground">Loading…</p>;
  if (instances.length === 0 && !adding) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="sentry-no-instances">
        No Sentry instances configured yet.
      </p>
    );
  }
  return (
    <div className="space-y-3">
      {instances.map((inst) => (
        <InstanceRow
          key={inst.id}
          instance={inst}
          editing={editingId === inst.id}
          onEdit={() => setEditingId(inst.id)}
          onCancelEdit={() => setEditingId(null)}
          onSaved={onSaved}
          onDelete={() => onDelete(inst)}
        />
      ))}
    </div>
  );
}

export function SentryConnectionSection() {
  const { toast } = useToast();
  const { instances, loading, reload } = useSentryInstances();
  const [adding, setAdding] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const onSaved = useCallback(() => {
    setAdding(false);
    setEditingId(null);
    void reload();
  }, [reload]);

  const handleDelete = useCallback(
    async (instance: SentryConfig) => {
      if (!confirm(`Remove Sentry instance "${instance.name}"?`)) return;
      try {
        await deleteSentryInstance(instance.id);
        toast({ description: "Sentry instance removed", variant: "success" });
        void reload();
      } catch (err) {
        toast({ description: describeDeleteError(err), variant: "error" });
      }
    },
    [reload, toast],
  );

  return (
    <SettingsSection
      icon={<IconBrandSentry className="h-5 w-5" />}
      title="Sentry integration"
      description="Connect Kandev to one or more Sentry instances with a user auth token. Credentials are stored encrypted server-side and shared across all workspaces."
      action={<EnabledPill />}
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <InstancesBody
            loading={loading}
            instances={instances}
            adding={adding}
            editingId={editingId}
            setEditingId={setEditingId}
            onSaved={onSaved}
            onDelete={handleDelete}
          />
          {adding && (
            <InstanceFormCard instance={null} onSaved={onSaved} onCancel={() => setAdding(false)} />
          )}
          {!adding && (
            <>
              <Separator />
              <Button
                type="button"
                variant="outline"
                onClick={() => setAdding(true)}
                className="cursor-pointer gap-1.5"
                data-testid="sentry-add-instance-button"
              >
                <IconPlus className="h-4 w-4" />
                Add instance
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </SettingsSection>
  );
}

export function SentryIntegrationPage() {
  return (
    <div className="space-y-8">
      <SentryConnectionSection />
      <SentryIssueWatchersSection />
    </div>
  );
}
