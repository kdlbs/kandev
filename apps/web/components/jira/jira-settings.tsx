"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { IconTicket } from "@tabler/icons-react";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { settingsCredentialClassName } from "@/components/settings/settings-control";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { TaskPresetsSection } from "@/components/jira/task-presets-section";
import { JiraIssueWatchersSection } from "@/components/jira/jira-issue-watchers-section";
import { JiraEnabledControl } from "@/components/jira/jira-enabled-control";
import { JiraActionBar } from "@/components/jira/jira-action-bar";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
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
import { useTranslation } from "react-i18next";
import {
  configToForm,
  deriveFormState,
  emptyForm,
  type FormState,
} from "@/components/jira/jira-form-state";
import {
  CookieExpiry,
  SecretHelp,
  secretCopy,
  secretPlaceholder,
} from "@/components/jira/jira-secret-help";
import { OAuthFields } from "@/components/jira/jira-oauth-fields";
import { INTEGRATION_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/integrations";

// Cloud sites live under this domain; shown as an example, not as prose, so it
// is interpolated rather than written into the catalog.
const ATLASSIAN_CLOUD_DOMAIN = "*.atlassian.net";

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
  if (auth === "oauth") return instance === "cloud";
  return false;
}

type FieldsRowProps = {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

type InstanceFieldsProps = FieldsRowProps & {
  setForm: Dispatch<SetStateAction<FormState>>;
};

function InstanceFields({ form, baseline, loading, setForm }: InstanceFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-instance">{t("jira:instanceType")}</Label>
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
          <SelectTrigger
            id="jira-instance"
            className="w-full cursor-pointer"
            data-settings-dirty={form.instanceType !== baseline.instanceType}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="cloud">{t("jira:atlassianCloud")}</SelectItem>
            <SelectItem value="server">{t("jira:serverDataCenter")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">
          {form.instanceType === "cloud"
            ? t("jira:sitesHostedAt", { domain: ATLASSIAN_CLOUD_DOMAIN })
            : t("jira:selfHostedJiraServerOrDataCenter")}
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="jira-project">{t("jira:defaultProjectKeyOptional")}</Label>
        <Input
          id="jira-project"
          data-testid="jira-project-input"
          placeholder="PROJ"
          value={form.defaultProjectKey}
          data-settings-dirty={form.defaultProjectKey !== baseline.defaultProjectKey}
          onChange={(e) =>
            setForm((prev) => ({ ...prev, defaultProjectKey: e.target.value.toUpperCase() }))
          }
          disabled={loading}
        />
      </div>
    </div>
  );
}

function SiteFields({ form, baseline, loading, update }: FieldsRowProps) {
  const { t } = useTranslation();
  const placeholder =
    form.instanceType === "server" ? "https://jira.your-company.com" : "https://acme.atlassian.net";
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-site">{t("jira:siteUrl")}</Label>
      <Input
        id="jira-site"
        data-testid="jira-site-input"
        placeholder={placeholder}
        value={form.siteUrl}
        data-settings-dirty={form.siteUrl !== baseline.siteUrl}
        onChange={(e) => update("siteUrl", e.target.value)}
        disabled={loading}
      />
    </div>
  );
}

function AuthFields({ form, baseline, loading, update }: FieldsRowProps) {
  const { t } = useTranslation();
  const showEmail = form.instanceType === "cloud" && form.authMethod === "api_token";
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="jira-auth">{t("jira:authenticationMethod")}</Label>
        <Select
          value={form.authMethod}
          onValueChange={(v) => update("authMethod", v as JiraAuthMethod)}
          disabled={loading}
        >
          <SelectTrigger
            id="jira-auth"
            className="w-full cursor-pointer"
            data-settings-dirty={form.authMethod !== baseline.authMethod}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {form.instanceType === "cloud" ? (
              <>
                <SelectItem value="api_token">{t("jira:apiTokenRecommended")}</SelectItem>
                <SelectItem value="oauth">{t("jira:oauth2")}</SelectItem>
                <SelectItem value="session_cookie">{t("jira:browserSessionCookie")}</SelectItem>
              </>
            ) : (
              <SelectItem value="pat">{t("jira:personalAccessToken")}</SelectItem>
            )}
          </SelectContent>
        </Select>
      </div>
      {showEmail ? (
        <div className="space-y-1.5">
          <Label htmlFor="jira-email">{t("jira:email")}</Label>
          <Input
            id="jira-email"
            data-testid="jira-email-input"
            type="email"
            placeholder="you@example.com"
            value={form.email}
            data-settings-dirty={form.email !== baseline.email}
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

type SecretFieldProps = FieldsRowProps & { hasSavedSecret: boolean };

type SecretFieldPropsWithExpiry = SecretFieldProps & { secretExpiresAt?: string | null };

function SecretField({
  form,
  baseline,
  loading,
  update,
  hasSavedSecret,
  secretExpiresAt,
}: SecretFieldPropsWithExpiry) {
  const { t } = useTranslation();
  const method = form.authMethod;
  const siteUrl = form.siteUrl.replace(/\/+$/, "");
  const patHref = siteUrl ? `${siteUrl}/secure/ViewProfile.jspa` : undefined;
  return (
    <div className="space-y-1.5">
      <Label htmlFor="jira-secret">
        {secretCopy(t)[method].label}
        {hasSavedSecret && (
          <span className="text-xs text-muted-foreground ml-2">{t("jira:savedLeaveBlank")}</span>
        )}
      </Label>
      <Input
        id="jira-secret"
        data-testid="jira-secret-input"
        type="password"
        placeholder={secretPlaceholder(t, method, hasSavedSecret)}
        value={form.secret}
        data-settings-dirty={form.secret !== baseline.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
        className={settingsCredentialClassName()}
      />
      {method === "session_cookie" && hasSavedSecret && secretExpiresAt && (
        <CookieExpiry expiresAt={secretExpiresAt} />
      )}
      <SecretHelp method={method} patHref={patHref} />
    </div>
  );
}

function TestResultAlert({ result }: { result: TestJiraConnectionResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {/* Both values are Jira's own reply — an account name and an upstream
            error message — so they travel through `values`, never the catalog. */}
        {result.ok
          ? t("jira:connectedAs", {
              name: result.displayName || result.email || result.accountId,
            })
          : t("jira:testFailed", { error: result.error })}
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

function useJiraConfigRefresh(workspaceId: string, setConfig: (cfg: JiraConfig | null) => void) {
  // Background refresh so the auth-health banner picks up new probe results
  // from the backend poller without requiring a page reload. We re-fetch the
  // config rather than the loud full `load()` to avoid flashing the form.
  useEffect(() => {
    const id = setInterval(() => {
      getJiraConfig({ workspaceId })
        .then((cfg) => setConfig(cfg))
        .catch(() => {
          /* transient failures are fine — next tick retries */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [workspaceId, setConfig]);
}

type JiraConfigMutationDeps = {
  workspaceId: string;
  form: FormState;
  setConfig: Dispatch<SetStateAction<JiraConfig | null>>;
  setForm: Dispatch<SetStateAction<FormState>>;
  setTestResult: (result: TestJiraConnectionResult | null) => void;
  setSaving: (saving: boolean) => void;
};

// The two callbacks that write the config and then re-seed the form, split out of
// useJiraSettings to keep each hook inside the function-length limit.
function useJiraConfigMutations({
  workspaceId,
  form,
  setConfig,
  setForm,
  setTestResult,
  setSaving,
}: JiraConfigMutationDeps) {
  const { t } = useTranslation();
  const { toast } = useToast();

  const handleSave = useCallback(async () => {
    const submitted = form;
    setSaving(true);
    try {
      const saved = await setJiraConfig(
        {
          siteUrl: form.siteUrl,
          email: form.email,
          authMethod: form.authMethod,
          instanceType: form.instanceType,
          defaultProjectKey: form.defaultProjectKey,
          secret: form.secret || undefined,
          clientId: form.clientId || undefined,
        },
        { workspaceId },
      );
      setConfig(saved);
      setForm((current) =>
        JSON.stringify(current) === JSON.stringify(submitted) ? configToForm(saved) : current,
      );
      // Clear any inline test result from the previous credentials so the
      // alert reflects only the currently-saved state.
      setTestResult(null);
      toast({ description: t("jira:jiraConfigurationSaved"), variant: "success" });
    } catch (err) {
      toast({ description: t("jira:saveFailed", { error: String(err) }), variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, setConfig, setForm, setTestResult, setSaving, t, toast]);

  const handleDelete = useCallback(async () => {
    try {
      await deleteJiraConfig({ workspaceId });
      setConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: t("jira:jiraConfigurationRemoved"), variant: "success" });
    } catch (err) {
      toast({ description: t("jira:deleteFailed", { error: String(err) }), variant: "error" });
    }
  }, [workspaceId, setConfig, setForm, setTestResult, t, toast]);

  return { handleSave, handleDelete };
}

function useJiraSettings(workspaceId: string) {
  const { t } = useTranslation();
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
      const cfg = await getJiraConfig({ workspaceId });
      setConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({
        description: t("jira:failedToLoadJiraConfig", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, t, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  useJiraConfigRefresh(workspaceId, setConfig);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testJiraConnection({ ...form }, { workspaceId });
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, form]);

  const { handleSave, handleDelete } = useJiraConfigMutations({
    workspaceId,
    form,
    setConfig,
    setForm,
    setTestResult,
    setSaving,
  });
  const discard = useCallback(() => setForm(configToForm(config)), [config]);

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
    discard,
    load,
  };
}

export function JiraConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const s = useJiraSettings(workspaceId);
  const baseline = configToForm(s.config);
  const {
    isOAuth,
    savedSecretMatchesMode,
    disableSave,
    disableTest,
    revision,
    dirty,
    invalidReason,
  } = deriveFormState(s, t);

  useSettingsSaveContributor({
    id: `jira-config:${workspaceId}`,
    revision,
    isDirty: dirty,
    canSave: !disableSave,
    invalidReason,
    save: s.handleSave,
    discard: s.discard,
  });

  return (
    <SettingsSection
      discoveryTargetId={INTEGRATION_SETTINGS_TARGETS.jira}
      icon={<IconTicket className="h-5 w-5" />}
      title={t("jira:jiraIntegration")}
      description={t("jira:connectThisWorkspaceToAtlassianCloud")}
      action={<JiraEnabledControl workspaceId={workspaceId} />}
    >
      <SettingsCard isDirty={dirty}>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={s.health} />
          <InstanceFields
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            update={s.update}
            setForm={s.setForm}
          />
          <SiteFields form={s.form} baseline={baseline} loading={s.loading} update={s.update} />
          <AuthFields form={s.form} baseline={baseline} loading={s.loading} update={s.update} />
          {isOAuth ? (
            <OAuthFields
              form={s.form}
              loading={s.loading}
              workspaceId={workspaceId}
              connected={savedSecretMatchesMode}
              tokenExpiresAt={s.config?.tokenExpiresAt ?? null}
              onConnected={() => void s.load()}
            />
          ) : (
            <SecretField
              form={s.form}
              baseline={baseline}
              loading={s.loading}
              update={s.update}
              hasSavedSecret={savedSecretMatchesMode}
              secretExpiresAt={s.config?.secretExpiresAt ?? null}
            />
          )}
          <TestResultAlert result={s.testResult} />
          <Separator />
          <JiraActionBar
            testing={s.testing}
            loading={s.loading}
            hasConfig={!!s.config}
            disableTest={disableTest}
            onTest={s.handleTest}
            onDelete={s.handleDelete}
          />
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}

export function JiraIntegrationPage({ workspaceId }: { workspaceId?: string } = {}) {
  return (
    <div className="space-y-8">
      <WorkspaceScopedSection workspaceId={workspaceId}>
        {(workspaceId) => <JiraConnectionSection key={workspaceId} workspaceId={workspaceId} />}
      </WorkspaceScopedSection>
      <JiraIssueWatchersSection />
      <TaskPresetsSection />
    </div>
  );
}
