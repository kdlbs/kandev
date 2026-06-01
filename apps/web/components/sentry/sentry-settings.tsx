"use client";

import { useCallback, useEffect, useState } from "react";
import { IconBrandSentry, IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
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
import {
  fetchSentryConfig,
  saveSentryConfig,
  deleteSentryConfig,
  testSentryConnection,
  listSentryOrganizations,
  listSentryProjects,
} from "@/lib/api/domains/sentry-api";
import {
  SENTRY_AUTH_METHOD,
  type SentryConfig,
  type SentryOrganization,
  type SentryProject,
  type TestSentryConnectionResult,
} from "@/lib/types/sentry";
import { SentryIssueWatchersSection } from "./sentry-issue-watchers-section";

type FormState = {
  defaultOrgSlug: string;
  defaultProjectSlug: string;
  secret: string;
};

const emptyForm: FormState = { defaultOrgSlug: "", defaultProjectSlug: "", secret: "" };

function configToForm(cfg: SentryConfig | null): FormState {
  if (!cfg) return emptyForm;
  return {
    defaultOrgSlug: cfg.defaultOrgSlug,
    defaultProjectSlug: cfg.defaultProjectSlug,
    secret: "",
  };
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

type OrgFieldProps = {
  form: FormState;
  loading: boolean;
  update: UpdateFn;
  orgs: string[];
  loadingProjects: boolean;
};

// OrgField is a dropdown populated from the API. It is only rendered once the
// token is validated, so there is no free-text fallback — the options always
// come from the org list the verified token can see.
function OrgField({ form, loading, update, orgs, loadingProjects }: OrgFieldProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="sentry-org">Default organization</Label>
      <Select
        value={form.defaultOrgSlug || "__none__"}
        onValueChange={(v) => {
          update("defaultOrgSlug", v === "__none__" ? "" : v);
          // The selected project may belong to a different org — clear it so the
          // project dropdown re-picks within the new org.
          update("defaultProjectSlug", "");
        }}
        disabled={loading || loadingProjects}
      >
        <SelectTrigger id="sentry-org" data-testid="sentry-org-input" className="w-full">
          <SelectValue placeholder="Choose an organization" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__">No default</SelectItem>
          {orgs.map((slug) => (
            <SelectItem key={slug} value={slug}>
              {slug}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

type ProjectSelectorProps = {
  form: FormState;
  loading: boolean;
  update: UpdateFn;
  projects: SentryProject[];
  loadingProjects: boolean;
};

function ProjectSelector({
  form,
  loading,
  update,
  projects,
  loadingProjects,
}: ProjectSelectorProps) {
  // Scope the list to the selected org so the dropdown only offers projects the
  // chosen org actually contains.
  const visibleProjects = form.defaultOrgSlug
    ? projects.filter((p) => p.orgSlug === form.defaultOrgSlug)
    : projects;
  return (
    <div className="space-y-1.5">
      <Label htmlFor="sentry-project">Default project (optional)</Label>
      <Select
        value={form.defaultProjectSlug || "__none__"}
        onValueChange={(v) => update("defaultProjectSlug", v === "__none__" ? "" : v)}
        disabled={loading || loadingProjects}
      >
        <SelectTrigger id="sentry-project" className="w-full">
          <SelectValue placeholder={loadingProjects ? "Loading projects…" : "Choose a project"} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__">No default</SelectItem>
          {visibleProjects.map((p) => (
            <SelectItem key={p.id} value={p.slug}>
              {p.name} ({p.slug})
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
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
  form: FormState;
  setConfig: (cfg: SentryConfig | null) => void;
  setForm: (form: FormState) => void;
  setTestResult: (r: TestSentryConnectionResult | null) => void;
};

function useSettingsActions({ form, setConfig, setForm, setTestResult }: SettingsActionsArgs) {
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testSentryConnection(form.secret || undefined);
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [form, setTestResult]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const saved = await saveSentryConfig({
        authMethod: SENTRY_AUTH_METHOD,
        defaultOrgSlug: form.defaultOrgSlug,
        defaultProjectSlug: form.defaultProjectSlug,
        secret: form.secret,
      });
      setConfig(saved);
      setForm(configToForm(saved));
      setTestResult(null);
      toast({ description: "Sentry configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [form, toast, setConfig, setForm, setTestResult]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Sentry configuration?")) return;
    try {
      await deleteSentryConfig();
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: "Sentry configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [toast, setConfig, setForm, setTestResult]);

  return { saving, testing, handleTest, handleSave, handleDelete };
}

function useProjectsLoader(hasSecret: boolean | undefined, lastOk: boolean | undefined) {
  const [projects, setProjects] = useState<SentryProject[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (!hasSecret) return;
    let cancelled = false;
    listSentryProjects()
      .then((res) => {
        if (cancelled) return;
        setProjects(res.projects ?? []);
        setError(null);
      })
      .catch((err) => {
        if (cancelled) return;
        setProjects([]);
        setError(String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [hasSecret, lastOk]);
  return {
    projects: projects ?? [],
    loadingProjects: projects === null && !!hasSecret,
    projectsError: error,
  };
}

function useOrgsLoader(hasSecret: boolean | undefined, lastOk: boolean | undefined) {
  const [organizations, setOrganizations] = useState<SentryOrganization[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (!hasSecret) return;
    let cancelled = false;
    listSentryOrganizations()
      .then((res) => {
        if (cancelled) return;
        setOrganizations(res.organizations ?? []);
        setError(null);
      })
      .catch((err) => {
        if (cancelled) return;
        setOrganizations([]);
        setError(String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [hasSecret, lastOk]);
  return { organizations: organizations ?? [], orgsError: error };
}

function useSentrySettings() {
  const { toast } = useToast();
  const [config, setConfig] = useState<SentryConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [testResult, setTestResult] = useState<TestSentryConnectionResult | null>(null);
  const health = configToHealth(config);
  const { projects, loadingProjects, projectsError } = useProjectsLoader(
    config?.hasSecret,
    config?.lastOk,
  );
  const { organizations, orgsError } = useOrgsLoader(config?.hasSecret, config?.lastOk);
  // Organizations the token can see, fetched from the backend. The saved org is
  // kept in the list so it stays selectable even if it is not currently
  // returned (e.g. the token's org membership changed).
  const orgs = (() => {
    const set = new Set(organizations.map((o) => o.slug).filter(Boolean));
    if (form.defaultOrgSlug) set.add(form.defaultOrgSlug);
    return Array.from(set).sort();
  })();

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = (await fetchSentryConfig()) ?? null;
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({ description: `Failed to load Sentry config: ${String(err)}`, variant: "error" });
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const id = setInterval(() => {
      fetchSentryConfig()
        .then((cfg) => setConfig(cfg ?? null))
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

  const { saving, testing, handleTest, handleSave, handleDelete } = useSettingsActions({
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
    projects,
    orgs,
    loadingProjects,
    orgsError,
    projectsError,
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

export function SentryConnectionSection() {
  const s = useSentrySettings();
  const missingSecret = !s.config?.hasSecret && !s.form.secret;
  const disableSave = s.saving || missingSecret;
  const disableTest = missingSecret;
  // Org/project selectors only make sense once the token is validated (saved
  // and the auth-health probe passed) — before that they'd be empty, so we hide
  // them entirely rather than show an unusable free-text input.
  const validated = !!s.config?.hasSecret && !!s.config?.lastOk;

  return (
    <SettingsSection
      icon={<IconBrandSentry className="h-5 w-5" />}
      title="Sentry integration"
      description="Connect Kandev to Sentry with a user auth token. Credentials are stored encrypted server-side and shared across all workspaces."
      action={<EnabledPill />}
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={s.health} />
          <SecretField
            form={s.form}
            loading={s.loading}
            update={s.update}
            hasSavedSecret={!!s.config?.hasSecret}
          />
          {validated && (
            <>
              {(s.orgsError || s.projectsError) && (
                <Alert variant="destructive">
                  <AlertDescription>
                    Couldn&apos;t load organizations or projects from Sentry. The auth token is
                    valid but likely missing the <code>org:read</code> / <code>project:read</code>{" "}
                    scopes — recreate it with those scopes (see the ⓘ next to Auth token).
                  </AlertDescription>
                </Alert>
              )}
              <OrgField
                form={s.form}
                loading={s.loading}
                update={s.update}
                orgs={s.orgs}
                loadingProjects={s.loadingProjects}
              />
              <ProjectSelector
                form={s.form}
                loading={s.loading}
                update={s.update}
                projects={s.projects}
                loadingProjects={s.loadingProjects}
              />
            </>
          )}
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

export function SentryIntegrationPage() {
  return (
    <div className="space-y-8">
      <SentryConnectionSection />
      <SentryIssueWatchersSection />
    </div>
  );
}
