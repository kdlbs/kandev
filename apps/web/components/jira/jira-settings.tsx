"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { IconTicket } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
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
  CookieExpiry,
  SecretHelp,
  secretCopy,
  secretPlaceholder,
} from "@/components/jira/jira-secret-help";
import { INTEGRATION_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/integrations";

// Cloud sites live under this domain; shown as an example, not as prose, so it
// is interpolated rather than written into the catalog.
const ATLASSIAN_CLOUD_DOMAIN = "*.atlassian.net";

type FormState = {
  siteUrl: string;
  email: string;
  authMethod: JiraAuthMethod;
  instanceType: JiraInstanceType;
  defaultProjectKey: string;
  secret: string;
  clientId: string;
};

const emptyForm: FormState = {
  siteUrl: "",
  email: "",
  authMethod: "api_token",
  instanceType: "cloud",
  defaultProjectKey: "",
  secret: "",
  clientId: "",
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
    clientId: cfg.clientId || "",
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

type OAuthFieldsProps = {
  form: FormState;
  loading: boolean;
  workspaceId: string;
  connected: boolean;
  tokenExpiresAt?: string | null;
  onConnected: () => void;
};

function OAuthFields({
  form,
  loading,
  workspaceId,
  connected,
  tokenExpiresAt,
  onConnected,
}: OAuthFieldsProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [connecting, setConnecting] = useState(false);
  const [showPasteDialog, setShowPasteDialog] = useState(false);
  const [pasteUrl, setPasteUrl] = useState("");
  const [completing, setCompleting] = useState(false);

  const handleConnect = useCallback(async () => {
    setConnecting(true);
    try {
      const params = new URLSearchParams({
        workspaceId,
        siteUrl: form.siteUrl,
      });
      const res = await fetch(`/api/v1/jira/oauth/start?${params}`);
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: "Request failed" }));
        toast({
          description: t("jira:oauthStartFailed", { error: err.error || String(err) }),
          variant: "error",
        });
        return;
      }
      const { authUrl } = await res.json();
      window.open(authUrl, "_blank");
      setShowPasteDialog(true);
    } catch (err) {
      toast({ description: t("jira:oauthStartFailed", { error: String(err) }), variant: "error" });
    } finally {
      setConnecting(false);
    }
  }, [workspaceId, form.siteUrl, t, toast]);

  const completeOAuth = useCallback(
    async (code: string, state: string) => {
      setCompleting(true);
      try {
        const res = await fetch(
          `/api/v1/jira/oauth/callback?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}&api=1`,
          { headers: { Accept: "application/json" } },
        );
        if (res.ok) {
          toast({ description: t("jira:oauthConnected"), variant: "success" });
          setShowPasteDialog(false);
          setPasteUrl("");
          onConnected();
        } else {
          const err = await res.json().catch(() => ({ error: "Failed" }));
          toast({
            description: t("jira:oauthStartFailed", { error: err.error || "Unknown error" }),
            variant: "error",
          });
          setShowPasteDialog(true);
        }
      } catch (err) {
        toast({
          description: t("jira:oauthStartFailed", { error: String(err) }),
          variant: "error",
        });
        setShowPasteDialog(true);
      } finally {
        setCompleting(false);
      }
    },
    [t, toast, onConnected],
  );

  const handlePasteComplete = useCallback(async () => {
    try {
      const url = new URL(pasteUrl);
      const code = url.searchParams.get("code");
      const state = url.searchParams.get("state");
      if (!code || !state) {
        toast({ description: t("jira:oauthPasteInvalid"), variant: "error" });
        return;
      }
      await completeOAuth(code, state);
    } catch {
      toast({ description: t("jira:oauthPasteInvalid"), variant: "error" });
    }
  }, [pasteUrl, t, toast, completeOAuth]);

  return (
    <div className="space-y-4">
      {connected && (
        <Alert>
          <AlertDescription>
            {t("jira:oauthConnected")}
            {tokenExpiresAt && (
              <span className="ml-2 text-xs text-muted-foreground">
                {t("jira:oauthTokenExpires", { date: new Date(tokenExpiresAt).toLocaleString() })}
              </span>
            )}
          </AlertDescription>
        </Alert>
      )}
      <Button
        type="button"
        onClick={handleConnect}
        disabled={connecting || loading || !form.siteUrl}
        className="cursor-pointer"
        data-testid="jira-oauth-connect"
      >
        {connecting ? t("jira:connecting") : t("jira:connectWithAtlassian")}
      </Button>
      <p className="text-xs text-muted-foreground">{t("jira:oauthDescription")}</p>

      {showPasteDialog && (
        <Alert>
          <AlertDescription className="space-y-3">
            <p className="font-medium">{t("jira:oauthPasteInstructions")}</p>
            <p className="text-xs">{t("jira:oauthPasteHelp")}</p>
            <Input
              type="text"
              placeholder="http://localhost:38429/api/v1/jira/oauth/callback?code=...&state=..."
              value={pasteUrl}
              onChange={(e) => setPasteUrl(e.target.value)}
              disabled={completing}
              data-testid="jira-oauth-paste-url"
            />
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                onClick={handlePasteComplete}
                disabled={completing || !pasteUrl}
                className="cursor-pointer"
              >
                {completing ? t("jira:connecting") : t("jira:oauthComplete")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setShowPasteDialog(false);
                  setPasteUrl("");
                }}
                className="cursor-pointer"
              >
                {t("jira:oauthCancel")}
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}

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

type ActionBarProps = {
  testing: boolean;
  loading: boolean;
  hasConfig: boolean;
  disableTest: boolean;
  onTest: () => void;
  onDelete: () => void;
};

function ActionBar({ testing, loading, hasConfig, disableTest, onTest, onDelete }: ActionBarProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest}
        className="cursor-pointer"
        title={disableTest ? t("jira:pasteATokenToTestTheConnection") : undefined}
        data-testid="jira-test-button"
      >
        {testing ? t("jira:testingConnection") : t("jira:testConnection")}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="jira-delete-button"
        >
          {t("jira:removeConfiguration")}
        </Button>
      )}
    </div>
  );
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
    if (!confirm(t("jira:removeJiraConfiguration"))) return;
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

function normalizeComparableSiteUrl(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, "");
  if (!trimmed) return "";
  return trimmed.includes("://") ? trimmed : `https://${trimmed}`;
}

// savedSecretMatches reports whether the saved secret can be reused against
// the current form values. Reuse is only safe when every identity component
// of the saved credential still matches: same auth method, same instance
// type, same Jira host, and — for Cloud api_token where the basic pair is
// email:token — the same email (case-insensitive). Otherwise the user could
// change the site URL or Cloud account and silently submit the previous
// token to a different host/account.
function savedSecretMatches(config: JiraConfig | null, form: FormState): boolean {
  if (!config?.hasSecret) return false;
  if (config.authMethod !== form.authMethod) return false;
  if ((config.instanceType || "cloud") !== form.instanceType) return false;
  if (normalizeComparableSiteUrl(config.siteUrl) !== normalizeComparableSiteUrl(form.siteUrl)) {
    return false;
  }
  if (form.authMethod !== "api_token") return true;
  return (config.email ?? "").toLowerCase() === form.email.toLowerCase();
}

// Compute derived form state for the Jira settings section.
function useJiraFormState(config: JiraConfig | null, form: FormState, loading: boolean) {
  const { t } = useTranslation();
  const baseline = configToForm(config);
  const savedSecretMatchesMode = savedSecretMatches(config, form);
  const isOAuth = form.authMethod === "oauth";
  const missingSecret = !isOAuth && !savedSecretMatchesMode && !form.secret;
  const oauthNeedsConnect = isOAuth && !config?.hasSecret;
  const emailRequired = form.instanceType === "cloud" && form.authMethod === "api_token";
  const disableSave = !form.siteUrl || (emailRequired && !form.email) || missingSecret;
  const disableTest = missingSecret || oauthNeedsConnect;
  const revision = JSON.stringify(form);
  const dirty = !loading && revision !== JSON.stringify(configToForm(config));
  let invalidReason: string | undefined;
  if (!form.siteUrl) invalidReason = t("jira:aJiraSiteUrlIsRequired");
  else if (emailRequired && !form.email) invalidReason = t("jira:anEmailAddressIsRequired");
  else if (missingSecret) invalidReason = t("jira:aCredentialIsRequired");
  return { baseline, isOAuth, disableSave, disableTest, revision, dirty, invalidReason };
}

export function JiraConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const s = useJiraSettings(workspaceId);
  const { baseline, isOAuth, disableSave, disableTest, revision, dirty, invalidReason } =
    useJiraFormState(s.config, s.form, s.loading);

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
              connected={!!s.config?.hasSecret}
              tokenExpiresAt={s.config?.tokenExpiresAt ?? null}
              onConnected={() => void load()}
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
          <ActionBar
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
