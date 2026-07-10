"use client";

import { useCallback, useEffect, useState } from "react";
import { IconBrandSentry, IconInfoCircle } from "@tabler/icons-react";
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
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  fetchSentryConfig,
  saveSentryConfig,
  deleteSentryConfig,
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
  url: string;
  secret: string;
};

const emptyForm: FormState = { url: SENTRY_DEFAULT_URL, secret: "" };

function configToForm(cfg: SentryConfig | null): FormState {
  // Only the instance URL and the (write-only) secret are editable here;
  // org/project are chosen per-watcher and per-browse, not stored install-wide.
  return { url: cfg?.url || SENTRY_DEFAULT_URL, secret: "" };
}

function saveLabel(saving: boolean, hasConfig: boolean): string {
  if (saving) return "Saving...";
  return hasConfig ? "Update" : "Save";
}

function configToHealth(config: SentryConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasSecret) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

type UpdateFn = <K extends keyof FormState>(key: K, value: FormState[K]) => void;

type UrlFieldProps = {
  form: FormState;
  loading: boolean;
  update: UpdateFn;
};

function UrlField({ form, loading, update }: UrlFieldProps) {
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
        disabled={loading}
      />
      <p className="text-xs text-muted-foreground">
        Base URL of your Sentry instance. Leave as {SENTRY_DEFAULT_URL} for Sentry SaaS, or point it
        at a self-hosted install (e.g. https://sentry.your-company.com).
      </p>
    </div>
  );
}

type SecretFieldProps = {
  form: FormState;
  loading: boolean;
  update: UpdateFn;
  hasSavedSecret: boolean;
};

function SecretField({ form, loading, update, hasSavedSecret }: SecretFieldProps) {
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
                <span className="opacity-70">
                  Issue &amp; Event — browse issues and run watchers
                </span>
              </li>
            </ul>
          </TooltipContent>
        </Tooltip>
      </div>
      <Input
        id="sentry-secret"
        data-testid="sentry-secret-input"
        type="password"
        placeholder={hasSavedSecret ? "••••••••" : "sntrys_..."}
        value={form.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
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

type ActionBarProps = {
  saving: boolean;
  testing: boolean;
  loading: boolean;
  hasConfig: boolean;
  disableSave: boolean;
  disableTest: boolean;
  onTest: () => void;
  onSave: () => void;
  onDelete: () => void;
};

function ActionBar({
  saving,
  testing,
  loading,
  hasConfig,
  disableSave,
  disableTest,
  onTest,
  onSave,
  onDelete,
}: ActionBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest}
        className="cursor-pointer"
        title={disableTest ? "Paste an auth token to test the connection" : undefined}
        data-testid="sentry-test-button"
      >
        {testing ? "Testing..." : "Test connection"}
      </Button>
      <Button
        type="button"
        onClick={onSave}
        disabled={disableSave}
        className="cursor-pointer"
        data-testid="sentry-save-button"
      >
        {saveLabel(saving, hasConfig)}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="sentry-delete-button"
        >
          Remove configuration
        </Button>
      )}
    </div>
  );
}

type SettingsActionsArgs = {
  workspaceId: string;
  form: FormState;
  setConfig: (cfg: SentryConfig | null) => void;
  setForm: (form: FormState) => void;
  setTestResult: (r: TestSentryConnectionResult | null) => void;
};

function useSettingsActions({
  workspaceId,
  form,
  setConfig,
  setForm,
  setTestResult,
}: SettingsActionsArgs) {
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testSentryConnection(form.secret || undefined, form.url || undefined, {
        workspaceId,
      });
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, form, setTestResult]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const saved = await saveSentryConfig(
        {
          authMethod: SENTRY_AUTH_METHOD,
          url: form.url,
          secret: form.secret,
        },
        { workspaceId },
      );
      setConfig(saved);
      setForm(configToForm(saved));
      setTestResult(null);
      toast({ description: "Sentry configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, toast, setConfig, setForm, setTestResult]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Sentry configuration?")) return;
    try {
      await deleteSentryConfig({ workspaceId });
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: "Sentry configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [workspaceId, toast, setConfig, setForm, setTestResult]);

  return { saving, testing, handleTest, handleSave, handleDelete };
}

function useSentrySettings(workspaceId: string) {
  const { toast } = useToast();
  const [config, setConfig] = useState<SentryConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [testResult, setTestResult] = useState<TestSentryConnectionResult | null>(null);
  const health = configToHealth(config);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = (await fetchSentryConfig({ workspaceId })) ?? null;
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({ description: `Failed to load Sentry config: ${String(err)}`, variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const id = setInterval(() => {
      fetchSentryConfig({ workspaceId })
        .then((cfg) => setConfig(cfg ?? null))
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [workspaceId]);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const { saving, testing, handleTest, handleSave, handleDelete } = useSettingsActions({
    workspaceId,
    form,
    setConfig,
    setForm,
    setTestResult,
  });

  return {
    config,
    form,
    loading,
    saving,
    testing,
    testResult,
    health,
    update,
    handleTest,
    handleSave,
    handleDelete,
  };
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

export function SentryConnectionSection({ workspaceId }: { workspaceId: string }) {
  const s = useSentrySettings(workspaceId);
  const missingSecret = !s.config?.hasSecret && !s.form.secret;
  const disableSave = s.saving || missingSecret;
  const disableTest = missingSecret;

  return (
    <SettingsSection
      icon={<IconBrandSentry className="h-5 w-5" />}
      title="Sentry integration"
      description="Connect this workspace to Sentry with a user auth token. Credentials are stored encrypted server-side for the selected workspace."
      action={<EnabledPill />}
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={s.health} />
          <UrlField form={s.form} loading={s.loading} update={s.update} />
          <SecretField
            form={s.form}
            loading={s.loading}
            update={s.update}
            hasSavedSecret={!!s.config?.hasSecret}
          />
          <TestResultAlert result={s.testResult} />
          <Separator />
          <ActionBar
            saving={s.saving}
            testing={s.testing}
            loading={s.loading}
            hasConfig={!!s.config}
            disableSave={disableSave}
            disableTest={disableTest}
            onTest={s.handleTest}
            onSave={s.handleSave}
            onDelete={s.handleDelete}
          />
        </CardContent>
      </Card>
    </SettingsSection>
  );
}

type SentryIntegrationPageProps = {
  workspaceId?: string;
};

export function SentryIntegrationPage({ workspaceId }: SentryIntegrationPageProps = {}) {
  return (
    <div className="space-y-8">
      <WorkspaceScopedSection workspaceId={workspaceId}>
        {(workspaceId) => <SentryConnectionSection key={workspaceId} workspaceId={workspaceId} />}
      </WorkspaceScopedSection>
      <SentryIssueWatchersSection />
    </div>
  );
}
