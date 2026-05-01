"use client";

import { useCallback, useEffect, useState } from "react";
import { IconTicket, IconCode } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { TaskPresetsSection } from "@/components/jira/task-presets-section";
import { JiraIssueWatchersSection } from "@/components/jira/jira-issue-watchers-section";
import { useJiraEnabled } from "@/hooks/domains/jira/use-jira-enabled";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getJiraConfig,
  setJiraConfig,
  deleteJiraConfig,
  testJiraConnection,
} from "@/lib/api/domains/jira-api";
import type { JiraAuthMethod, JiraConfig, TestJiraConnectionResult } from "@/lib/types/jira";

// Session cookies are HttpOnly so document.cookie can't read them, but
// DevTools → Application → Cookies surfaces them in plain text. Users copy
// the Value cell of a single row; the backend wraps it under both
// cloud.session.token and tenant.session.token so a single paste works for
// password accounts and SSO tenants.
const COOKIE_INSTRUCTIONS = `Open DevTools (Cmd+Opt+I / Ctrl+Shift+I) on your Atlassian tab →
Application tab → Storage → Cookies → https://*.atlassian.net →
find the row named "cloud.session.token" (or "tenant.session.token"
on SSO tenants) → copy the Value cell → paste it below.
Don't include the cookie name or any "=" — just the token value.`;

type FormState = {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  defaultProjectKey: string;
  secret: string;
};

const emptyForm: FormState = {
  siteUrl: "",
  email: "",
  authMethod: "api_token",
  defaultProjectKey: "",
  secret: "",
};

function configToForm(cfg: JiraConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    siteUrl: cfg.siteUrl,
    email: cfg.email,
    authMethod: cfg.authMethod,
    defaultProjectKey: cfg.defaultProjectKey,
    secret: "",
  };
}

function saveLabel(saving: boolean, hasConfig: boolean): string {
  if (saving) return "Saving...";
  return hasConfig ? "Update" : "Save";
}

type FieldsRowProps = {
  form: FormState;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

function SiteFields({ form, loading, update }: FieldsRowProps) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-site">Site URL</Label>
        <Input
          id="jira-site"
          placeholder="https://acme.atlassian.net"
          value={form.siteUrl}
          onChange={(e) => update("siteUrl", e.target.value)}
          disabled={loading}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="jira-project">Default project key (optional)</Label>
        <Input
          id="jira-project"
          placeholder="PROJ"
          value={form.defaultProjectKey}
          onChange={(e) => update("defaultProjectKey", e.target.value.toUpperCase())}
          disabled={loading}
        />
      </div>
    </div>
  );
}

function AuthFields({ form, loading, update }: FieldsRowProps) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-auth">Authentication method</Label>
        <Select
          value={form.authMethod}
          onValueChange={(v) => update("authMethod", v as JiraAuthMethod)}
          disabled={loading}
        >
          <SelectTrigger id="jira-auth" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="api_token">API token (recommended)</SelectItem>
            <SelectItem value="session_cookie">Browser session cookie</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="jira-email">
          Email {form.authMethod === "session_cookie" && "(optional)"}
        </Label>
        <Input
          id="jira-email"
          type="email"
          placeholder="you@example.com"
          value={form.email}
          onChange={(e) => update("email", e.target.value)}
          disabled={loading}
        />
      </div>
    </div>
  );
}

function SessionSnippet() {
  const [show, setShow] = useState(false);
  return (
    <div className="text-xs text-muted-foreground space-y-2">
      <button
        type="button"
        onClick={() => setShow((v) => !v)}
        className="inline-flex items-center gap-1 underline cursor-pointer"
      >
        <IconCode className="h-3 w-3" />
        {show ? "Hide" : "Show"} how to copy the session token
      </button>
      {show && (
        <pre className="bg-muted rounded p-3 text-[11px] overflow-x-auto whitespace-pre-wrap">
          <code>{COOKIE_INSTRUCTIONS}</code>
        </pre>
      )}
    </div>
  );
}

type SecretFieldProps = FieldsRowProps & { hasSavedSecret: boolean };

function secretPlaceholder(isApiToken: boolean, hasSavedSecret: boolean): string {
  if (hasSavedSecret) return "••••••••";
  return isApiToken ? "paste token here" : "paste cloud.session.token value";
}

function formatExpiry(expiresAt: string): { label: string; tone: "ok" | "warn" | "danger" } {
  const diffMs = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(diffMs)) return { label: "Expiry unknown", tone: "warn" };
  if (diffMs <= 0) return { label: "Cookie expired — paste a fresh one", tone: "danger" };
  const hours = diffMs / (60 * 60 * 1000);
  if (hours < 24) {
    const h = Math.max(1, Math.round(hours));
    return { label: `Cookie expires in ${h}h`, tone: "danger" };
  }
  const days = Math.round(hours / 24);
  return {
    label: `Cookie expires in ${days} day${days === 1 ? "" : "s"}`,
    tone: days < 7 ? "warn" : "ok",
  };
}

const TONE_CLASSES: Record<"ok" | "warn" | "danger", string> = {
  ok: "text-muted-foreground",
  warn: "text-amber-600 dark:text-amber-400",
  danger: "text-destructive",
};

function CookieExpiry({ expiresAt }: { expiresAt: string }) {
  const { label, tone } = formatExpiry(expiresAt);
  const absolute = new Date(expiresAt).toLocaleString();
  return (
    <p className={`text-xs ${TONE_CLASSES[tone]}`} title={absolute}>
      {label}
    </p>
  );
}

type SecretFieldPropsWithExpiry = SecretFieldProps & { secretExpiresAt?: string | null };

function SecretField({
  form,
  loading,
  update,
  hasSavedSecret,
  secretExpiresAt,
}: SecretFieldPropsWithExpiry) {
  const isApiToken = form.authMethod === "api_token";
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-secret">
        {isApiToken ? "API token" : "Session token value"}
        {hasSavedSecret && (
          <span className="text-xs text-muted-foreground ml-2">
            (saved — leave blank to keep the current value)
          </span>
        )}
      </Label>
      <Input
        id="jira-secret"
        type="password"
        placeholder={secretPlaceholder(isApiToken, hasSavedSecret)}
        value={form.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
      {!isApiToken && hasSavedSecret && secretExpiresAt && (
        <CookieExpiry expiresAt={secretExpiresAt} />
      )}
      {isApiToken && (
        <p className="text-xs text-muted-foreground">
          Create a token at{" "}
          <a
            className="underline"
            href="https://id.atlassian.com/manage-profile/security/api-tokens"
            target="_blank"
            rel="noreferrer"
          >
            id.atlassian.com/manage-profile/security/api-tokens
          </a>
        </p>
      )}
      {!isApiToken && <SessionSnippet />}
    </div>
  );
}

function TestResultAlert({ result }: { result: TestJiraConnectionResult | null }) {
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {result.ok
          ? `Connected as ${result.displayName || result.email || result.accountId}`
          : `Failed: ${result.error}`}
      </AlertDescription>
    </Alert>
  );
}

function configToHealth(config: JiraConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasSecret) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
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
        title={disableTest ? "Paste a token to test the connection" : undefined}
      >
        {testing ? "Testing..." : "Test connection"}
      </Button>
      <Button type="button" onClick={onSave} disabled={disableSave} className="cursor-pointer">
        {saveLabel(saving, hasConfig)}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
        >
          Remove configuration
        </Button>
      )}
    </div>
  );
}

type JiraSettingsProps = {
  workspaceId: string;
};

function useJiraSettings(workspaceId: string) {
  const { toast } = useToast();
  const [config, setConfig] = useState<JiraConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestJiraConnectionResult | null>(null);
  const health = configToHealth(config);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await getJiraConfig(workspaceId);
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({ description: `Failed to load Jira config: ${String(err)}`, variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  // Background refresh so the auth-health banner picks up new probe results
  // from the backend poller without requiring a page reload. We re-fetch the
  // config rather than the loud full `load()` to avoid flashing the form.
  useEffect(() => {
    const id = setInterval(() => {
      getJiraConfig(workspaceId)
        .then((cfg) => setConfig(cfg))
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

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testJiraConnection({ workspaceId, ...form });
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, form]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const saved = await setJiraConfig({
        workspaceId,
        siteUrl: form.siteUrl,
        email: form.email,
        authMethod: form.authMethod,
        defaultProjectKey: form.defaultProjectKey,
        secret: form.secret || undefined,
      });
      setConfig(saved);
      setForm(configToForm(saved));
      // Clear any inline test result from the previous credentials so the
      // alert reflects only the currently-saved state.
      setTestResult(null);
      toast({ description: "Jira configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, toast]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Jira configuration for this workspace?")) return;
    try {
      await deleteJiraConfig(workspaceId);
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: "Jira configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [workspaceId, toast]);

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

function EnabledPill({ workspaceId }: { workspaceId: string }) {
  const { enabled, setEnabled } = useJiraEnabled(workspaceId);
  return (
    <div className="flex items-center gap-2 rounded-full border bg-muted/30 px-3 py-1">
      <Switch
        id="jira-enabled"
        checked={enabled}
        onCheckedChange={setEnabled}
        className="cursor-pointer"
      />
      <Label htmlFor="jira-enabled" className="text-xs cursor-pointer">
        {enabled ? "Enabled" : "Disabled"}
      </Label>
    </div>
  );
}

export function JiraSettings({ workspaceId }: JiraSettingsProps) {
  const s = useJiraSettings(workspaceId);
  const missingSecret = !s.config?.hasSecret && !s.form.secret;
  const disableSave =
    s.saving ||
    !s.form.siteUrl ||
    (s.form.authMethod === "api_token" && !s.form.email) ||
    missingSecret;
  const disableTest = missingSecret;

  return (
    <div className="space-y-8">
      <SettingsSection
        icon={<IconTicket className="h-5 w-5" />}
        title="Jira integration"
        description="Connect this workspace to an Atlassian Cloud site. Credentials are stored encrypted server-side."
        action={<EnabledPill workspaceId={workspaceId} />}
      >
        <Card>
          <CardContent className="space-y-4 pt-6">
            <IntegrationAuthStatusBanner health={s.health} />
            <SiteFields form={s.form} loading={s.loading} update={s.update} />
            <AuthFields form={s.form} loading={s.loading} update={s.update} />
            <SecretField
              form={s.form}
              loading={s.loading}
              update={s.update}
              hasSavedSecret={!!s.config?.hasSecret}
              secretExpiresAt={s.config?.secretExpiresAt ?? null}
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
      {s.config?.hasSecret && <JiraIssueWatchersSection workspaceId={workspaceId} />}
      <TaskPresetsSection />
    </div>
  );
}
