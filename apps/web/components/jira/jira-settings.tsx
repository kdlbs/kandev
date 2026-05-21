"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
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
import type {
  JiraAuthMethod,
  JiraConfig,
  JiraInstanceType,
  TestJiraConnectionResult,
} from "@/lib/types/jira";

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
  instanceType: JiraInstanceType;
  defaultProjectKey: string;
  secret: string;
};

const emptyForm: FormState = {
  siteUrl: "",
  email: "",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "",
  secret: "",
};

function configToForm(cfg: JiraConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    siteUrl: cfg.siteUrl,
    email: cfg.email,
    authMethod: cfg.authMethod,
    // Legacy rows written before Server/DC support carry an empty instanceType;
    // default to cloud so the dropdown has a valid selection.
    instanceType: cfg.instanceType || "cloud",
    defaultProjectKey: cfg.defaultProjectKey,
    secret: "",
  };
}

// defaultAuthForInstance returns the canonical auth method for an instance
// type. Used when the user switches Instance type and the current auth method
// is no longer valid for the new type (e.g. PAT picked for Cloud).
function defaultAuthForInstance(instance: JiraInstanceType): JiraAuthMethod {
  return instance === "server" ? "pat" : "api_token";
}

// authAllowedForInstance reports whether an auth method is allowed for a given
// instance type. Mirrors the backend validation so the user can't submit an
// invalid combination. session_cookie is Cloud-only today because the backend
// wraps the secret under cloud.session.token / tenant.session.token cookie
// names — Server/DC uses JSESSIONID, so the wrapping is a no-op there until we
// add a Server-aware path.
function authAllowedForInstance(auth: JiraAuthMethod, instance: JiraInstanceType): boolean {
  if (auth === "api_token") return instance === "cloud";
  if (auth === "pat") return instance === "server";
  if (auth === "session_cookie") return instance === "cloud";
  return false;
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

type InstanceFieldsProps = FieldsRowProps & {
  setForm: Dispatch<SetStateAction<FormState>>;
};

function InstanceFields({ form, loading, setForm }: InstanceFieldsProps) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-instance">Instance type</Label>
        <Select
          value={form.instanceType}
          onValueChange={(v) => {
            const next = v as JiraInstanceType;
            setForm((prev) => {
              // Switching instance type changes which auth methods are valid;
              // if the current one would become invalid, swap it for the
              // canonical default for the new instance.
              const auth = authAllowedForInstance(prev.authMethod, next)
                ? prev.authMethod
                : defaultAuthForInstance(next);
              return { ...prev, instanceType: next, authMethod: auth };
            });
          }}
          disabled={loading}
        >
          <SelectTrigger id="jira-instance" className="w-full cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="cloud">Atlassian Cloud</SelectItem>
            <SelectItem value="server">Server / Data Center</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {form.instanceType === "cloud"
            ? "Sites hosted at *.atlassian.net."
            : "Self-hosted Jira (Server or Data Center)."}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="jira-project">Default project key (optional)</Label>
        <Input
          id="jira-project"
          data-testid="jira-project-input"
          placeholder="PROJ"
          value={form.defaultProjectKey}
          onChange={(e) => setForm((prev) => ({ ...prev, defaultProjectKey: e.target.value.toUpperCase() }))}
          disabled={loading}
        />
      </div>
    </div>
  );
}

function SiteFields({ form, loading, update }: FieldsRowProps) {
  const placeholder = form.instanceType === "server"
    ? "https://jira.your-company.com"
    : "https://acme.atlassian.net";
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-site">Site URL</Label>
      <Input
        id="jira-site"
        data-testid="jira-site-input"
        placeholder={placeholder}
        value={form.siteUrl}
        onChange={(e) => update("siteUrl", e.target.value)}
        disabled={loading}
      />
    </div>
  );
}

function AuthFields({ form, loading, update }: FieldsRowProps) {
  const showEmail = form.instanceType === "cloud" && form.authMethod === "api_token";
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-auth">Authentication method</Label>
        <Select
          value={form.authMethod}
          onValueChange={(v) => update("authMethod", v as JiraAuthMethod)}
          disabled={loading}
        >
          <SelectTrigger id="jira-auth" className="w-full cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {form.instanceType === "cloud" ? (
              <>
                <SelectItem value="api_token">API token (recommended)</SelectItem>
                <SelectItem value="session_cookie">Browser session cookie</SelectItem>
              </>
            ) : (
              <SelectItem value="pat">Personal Access Token</SelectItem>
            )}
          </SelectContent>
        </Select>
      </div>
      {showEmail ? (
        <div className="space-y-1.5">
          <Label htmlFor="jira-email">Email</Label>
          <Input
            id="jira-email"
            data-testid="jira-email-input"
            type="email"
            placeholder="you@example.com"
            value={form.email}
            onChange={(e) => update("email", e.target.value)}
            disabled={loading}
          />
        </div>
      ) : (
        // Keep the grid balanced so the auth select doesn't span both columns.
        <div aria-hidden className="hidden sm:block" />
      )}
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

function secretLabel(method: JiraAuthMethod): string {
  switch (method) {
    case "api_token":
      return "API token";
    case "pat":
      return "Personal Access Token";
    case "session_cookie":
    default:
      return "Session token value";
  }
}

function secretPlaceholder(method: JiraAuthMethod, hasSavedSecret: boolean): string {
  if (hasSavedSecret) return "••••••••";
  switch (method) {
    case "api_token":
      return "paste API token here";
    case "pat":
      return "paste personal access token here";
    case "session_cookie":
    default:
      return "paste cloud.session.token value";
  }
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
  const method = form.authMethod;
  const siteUrl = form.siteUrl.replace(/\/+$/, "");
  const patHref = siteUrl ? `${siteUrl}/secure/ViewProfile.jspa` : undefined;
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-secret">
        {secretLabel(method)}
        {hasSavedSecret && (
          <span className="text-xs text-muted-foreground ml-2">
            (saved — leave blank to keep the current value)
          </span>
        )}
      </Label>
      <Input
        id="jira-secret"
        data-testid="jira-secret-input"
        type="password"
        placeholder={secretPlaceholder(method, hasSavedSecret)}
        value={form.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
      {method === "session_cookie" && hasSavedSecret && secretExpiresAt && (
        <CookieExpiry expiresAt={secretExpiresAt} />
      )}
      {method === "api_token" && (
        <p className="text-xs text-muted-foreground">
          Create a token at{" "}
          <a
            className="underline cursor-pointer"
            href="https://id.atlassian.com/manage-profile/security/api-tokens"
            target="_blank"
            rel="noreferrer"
          >
            id.atlassian.com/manage-profile/security/api-tokens
          </a>
        </p>
      )}
      {method === "pat" && (
        <p className="text-xs text-muted-foreground">
          Create a Personal Access Token from your Jira profile
          {patHref ? (
            <>
              {" "}
              (
              <a className="underline cursor-pointer" href={patHref} target="_blank" rel="noreferrer">
                {patHref}
              </a>
              ){" "}
            </>
          ) : (
            " "
          )}
          → Personal Access Tokens. Required scopes: read & write.
        </p>
      )}
      {method === "session_cookie" && <SessionSnippet />}
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
        data-testid="jira-test-button"
      >
        {testing ? "Testing..." : "Test connection"}
      </Button>
      <Button
        type="button"
        onClick={onSave}
        disabled={disableSave}
        className="cursor-pointer"
        data-testid="jira-save-button"
      >
        {saveLabel(saving, hasConfig)}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="jira-delete-button"
        >
          Remove configuration
        </Button>
      )}
    </div>
  );
}

function useJiraSettings() {
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
      const cfg = await getJiraConfig();
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({ description: `Failed to load Jira config: ${String(err)}`, variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  // Background refresh so the auth-health banner picks up new probe results
  // from the backend poller without requiring a page reload. We re-fetch the
  // config rather than the loud full `load()` to avoid flashing the form.
  useEffect(() => {
    const id = setInterval(() => {
      getJiraConfig()
        .then((cfg) => setConfig(cfg))
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, []);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testJiraConnection({ ...form });
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [form]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const saved = await setJiraConfig({
        siteUrl: form.siteUrl,
        email: form.email,
        authMethod: form.authMethod,
        instanceType: form.instanceType,
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
  }, [form, toast]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Jira configuration?")) return;
    try {
      await deleteJiraConfig();
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: "Jira configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [toast]);

  return {
    config,
    form,
    setForm,
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
  const { enabled, setEnabled } = useJiraEnabled();
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

// JiraConnectionSection holds the install-wide credentials form. Watchers and
// task presets live elsewhere because they scope per workspace.
export function JiraConnectionSection() {
  const s = useJiraSettings();
  // The stored secret is only safe to reuse when every identity component of
  // the saved credential still matches: same auth method, same (normalized)
  // instance type, same Jira host, and — for Cloud api_token where the basic
  // pair is email:token — the same email. Otherwise the user could change the
  // site URL or Cloud account and silently submit the previous token to a
  // different host/account.
  const savedInstance = s.config?.instanceType || "cloud";
  const savedSiteUrl = (s.config?.siteUrl ?? "").replace(/\/+$/, "");
  const currentSiteUrl = s.form.siteUrl.replace(/\/+$/, "");
  const savedSecretMatchesMode =
    !!s.config?.hasSecret &&
    s.config?.authMethod === s.form.authMethod &&
    savedInstance === s.form.instanceType &&
    savedSiteUrl === currentSiteUrl &&
    (s.form.authMethod !== "api_token" || s.config?.email === s.form.email);
  const missingSecret = !savedSecretMatchesMode && !s.form.secret;
  // Email is only required for the Cloud + api_token combination. Server PAT
  // and session cookies authenticate the user out of the token itself.
  const emailRequired =
    s.form.instanceType === "cloud" && s.form.authMethod === "api_token";
  const disableSave =
    s.saving ||
    !s.form.siteUrl ||
    (emailRequired && !s.form.email) ||
    missingSecret;
  const disableTest = missingSecret;

  return (
    <SettingsSection
      icon={<IconTicket className="h-5 w-5" />}
      title="Jira integration"
      description="Connect Kandev to Atlassian Cloud or a self-hosted Jira Server / Data Center instance. Credentials are stored encrypted server-side and shared across all workspaces."
      action={<EnabledPill />}
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={s.health} />
          <InstanceFields
            form={s.form}
            loading={s.loading}
            update={s.update}
            setForm={s.setForm}
          />
          <SiteFields form={s.form} loading={s.loading} update={s.update} />
          <AuthFields form={s.form} loading={s.loading} update={s.update} />
          <SecretField
            form={s.form}
            loading={s.loading}
            update={s.update}
            hasSavedSecret={savedSecretMatchesMode}
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
  );
}

// JiraIntegrationPage is the install-wide settings surface mounted at
// /settings/integrations/jira. The connection form and task-prompt presets
// are global; the watchers section lists every watch across every workspace
// in a single table, with workspace selection happening inside the create
// dialog.
export function JiraIntegrationPage() {
  return (
    <div className="space-y-8">
      <JiraConnectionSection />
      <JiraIssueWatchersSection />
      <TaskPresetsSection />
    </div>
  );
}
