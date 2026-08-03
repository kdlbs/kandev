"use client";

import { useCallback, useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { IconHexagon } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useToast } from "@/components/toast-provider";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { useLinearEnabled } from "@/hooks/domains/linear/use-linear-enabled";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import {
  getLinearConfig,
  setLinearConfig,
  deleteLinearConfig,
  testLinearConnection,
  listLinearTeams,
} from "@/lib/api/domains/linear-api";
import type { LinearConfig, LinearTeam, TestLinearConnectionResult } from "@/lib/types/linear";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { LinearIssueWatchersSection } from "./linear-issue-watchers-section";

type FormState = {
  defaultTeamKey: string;
  secret: string;
};

const emptyForm: FormState = { defaultTeamKey: "", secret: "" };

// Not copy. The mask is a glyph run, `lin_api_...` is the literal shape of a
// Linear personal API key, and the link is a Linear-owned URL — its visible
// text is the same URL with the scheme trimmed. All four would be corrupted by
// the pseudo-locale into something the user cannot match against Linear's own
// UI, so they stay out of the catalog.
const SECRET_MASK = "••••••••";
const SECRET_EXAMPLE = "lin_api_...";
const API_KEY_SETTINGS_URL = "https://linear.app/settings/account/security";
const API_KEY_SETTINGS_LABEL = "linear.app/settings/account/security";

function configToForm(cfg: LinearConfig | null): FormState {
  if (!cfg) return emptyForm;
  return { defaultTeamKey: cfg.defaultTeamKey, secret: "" };
}

type FieldsRowProps = {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
  hasSavedSecret: boolean;
  teams: LinearTeam[];
  loadingTeams: boolean;
};

function SecretField({
  form,
  baseline,
  loading,
  update,
  hasSavedSecret,
}: Omit<FieldsRowProps, "teams" | "loadingTeams">) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label htmlFor="linear-secret">
        {t("linear:apiKey")}
        {hasSavedSecret && (
          <span className="text-xs text-muted-foreground ml-2">{t("linear:savedLeaveBlank")}</span>
        )}
      </Label>
      <Input
        id="linear-secret"
        data-testid="linear-secret-input"
        type="password"
        placeholder={hasSavedSecret ? SECRET_MASK : SECRET_EXAMPLE}
        value={form.secret}
        data-settings-dirty={form.secret !== baseline.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
      <p className="text-xs text-muted-foreground">
        {t("linear:createPersonalApiKeyAt")}{" "}
        <a
          className="underline cursor-pointer"
          href={API_KEY_SETTINGS_URL}
          target="_blank"
          rel="noreferrer"
        >
          {API_KEY_SETTINGS_LABEL}
        </a>
      </p>
    </div>
  );
}

function TeamSelector({ form, baseline, loading, update, teams, loadingTeams }: FieldsRowProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label htmlFor="linear-team">{t("linear:defaultTeamOptional")}</Label>
      <Select
        value={form.defaultTeamKey || "__none__"}
        onValueChange={(v) => update("defaultTeamKey", v === "__none__" ? "" : v)}
        disabled={loading || loadingTeams}
      >
        <SelectTrigger
          id="linear-team"
          className="w-full"
          data-settings-dirty={form.defaultTeamKey !== baseline.defaultTeamKey}
        >
          <SelectValue
            placeholder={loadingTeams ? t("linear:loadingTeams") : t("linear:chooseATeam")}
          />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__">{t("linear:noDefault")}</SelectItem>
          {/* Renamed from `t` so it cannot shadow the translation function
              above; team names and keys are Linear API data, not copy. */}
          {teams.map((team) => (
            <SelectItem key={team.id} value={team.key}>
              {team.name} ({team.key})
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// The identity and the org name are Linear API data, so they are interpolated
// rather than written into the catalog — a locale (pseudo included) must not
// rewrite the account the user is checking they connected as.
//
// Every field on the result is optional, so each interpolated value needs a
// fallback: i18next leaves an unmatched placeholder in place, so a partial
// response would render the literal "{{name}}" to the user rather than the
// old template's "undefined".
function testResultMessage(t: TFunction, result: TestLinearConnectionResult): string {
  if (!result.ok) {
    return t("linear:testFailed", { error: result.error || t("linear:unknownError") });
  }
  const name = result.displayName || result.email || result.userId || t("linear:unknownAccount");
  return result.orgName
    ? t("linear:connectedAsWithOrg", { name, org: result.orgName })
    : t("linear:connectedAs", { name });
}

function TestResultAlert({ result }: { result: TestLinearConnectionResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>{testResultMessage(t, result)}</AlertDescription>
    </Alert>
  );
}

function configToHealth(config: LinearConfig | null): IntegrationAuthHealth | null {
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
        title={disableTest ? t("linear:pasteAnApiKeyToTest") : undefined}
        data-testid="linear-test-button"
      >
        {testing ? t("linear:testing") : t("linear:testConnection")}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="linear-delete-button"
        >
          {t("linear:removeConfiguration")}
        </Button>
      )}
    </div>
  );
}

type SettingsActionsArgs = {
  workspaceId: string;
  form: FormState;
  setConfig: (cfg: LinearConfig | null) => void;
  setBaselineConfig: (cfg: LinearConfig | null) => void;
  setForm: Dispatch<SetStateAction<FormState>>;
  setTestResult: (r: TestLinearConnectionResult | null) => void;
};

function useSettingsActions({
  workspaceId,
  form,
  setConfig,
  setBaselineConfig,
  setForm,
  setTestResult,
}: SettingsActionsArgs) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testLinearConnection(
        {
          authMethod: "api_key",
          secret: form.secret || undefined,
        },
        { workspaceId },
      );
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, form, setTestResult]);

  const handleSave = useCallback(async () => {
    const submitted = form;
    setSaving(true);
    try {
      const saved = await setLinearConfig(
        {
          authMethod: "api_key",
          defaultTeamKey: form.defaultTeamKey,
          secret: form.secret || undefined,
        },
        { workspaceId },
      );
      setConfig(saved);
      setBaselineConfig(saved);
      setForm((current) =>
        JSON.stringify(current) === JSON.stringify(submitted) ? configToForm(saved) : current,
      );
      setTestResult(null);
      toast({ description: t("linear:configurationSaved"), variant: "success" });
    } catch (err) {
      toast({ description: t("linear:saveFailed", { error: String(err) }), variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, t, toast, setConfig, setBaselineConfig, setForm, setTestResult]);

  const handleDelete = useCallback(async () => {
    if (!confirm(t("linear:removeLinearConfigurationConfirm"))) return;
    try {
      await deleteLinearConfig({ workspaceId });
      setConfig(null);
      setBaselineConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: t("linear:configurationRemoved"), variant: "success" });
    } catch (err) {
      toast({ description: t("linear:deleteFailed", { error: String(err) }), variant: "error" });
    }
  }, [workspaceId, t, toast, setConfig, setBaselineConfig, setForm, setTestResult]);

  return { saving, testing, handleTest, handleSave, handleDelete };
}

function useTeamsLoader(
  workspaceId: string,
  hasSecret: boolean | undefined,
  lastOk: boolean | undefined,
) {
  // `teams === null` means "no fetch attempt yet", so the dropdown can show a
  // "Loading…" placeholder without us calling setState synchronously inside
  // the effect (which the lint rule forbids). Once a fetch settles we always
  // store an array, even if empty.
  const [teams, setTeams] = useState<LinearTeam[] | null>(null);
  // Fetch teams once a working configuration exists. Skips when there's no
  // saved secret (the API call would 503). Stale teams from a previous save
  // remain visible after deletion, but the dropdown is gated on hasSecret so
  // the user never sees them.
  useEffect(() => {
    if (!hasSecret) return;
    let cancelled = false;
    listLinearTeams({ workspaceId })
      .then((res) => {
        if (!cancelled) setTeams(res.teams ?? []);
      })
      .catch(() => {
        if (!cancelled) setTeams([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, hasSecret, lastOk]);
  return { teams: teams ?? [], loadingTeams: teams === null && !!hasSecret };
}

function useLinearSettings(workspaceId: string) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [config, setConfig] = useState<LinearConfig | null>(null);
  const [baselineConfig, setBaselineConfig] = useState<LinearConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [testResult, setTestResult] = useState<TestLinearConnectionResult | null>(null);
  const health = configToHealth(config);
  const { teams, loadingTeams } = useTeamsLoader(workspaceId, config?.hasSecret, config?.lastOk);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const cfg = await getLinearConfig({ workspaceId });
      setConfig(cfg);
      setBaselineConfig(cfg);
      setForm(configToForm(cfg));
    } catch (err) {
      toast({
        description: t("linear:failedToLoadConfig", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [workspaceId, t, toast]);

  useEffect(() => {
    void load();
  }, [load]);

  // Background refresh so the auth-health banner picks up new probe results.
  useEffect(() => {
    const id = setInterval(() => {
      getLinearConfig({ workspaceId })
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
  const discard = useCallback(() => setForm(configToForm(baselineConfig)), [baselineConfig]);

  const { saving, testing, handleTest, handleSave, handleDelete } = useSettingsActions({
    workspaceId,
    form,
    setConfig,
    setBaselineConfig,
    setForm,
    setTestResult,
  });

  return {
    config,
    baselineConfig,
    form,
    loading,
    saving,
    testing,
    testResult,
    health,
    teams,
    loadingTeams,
    update,
    discard,
    handleTest,
    handleSave,
    handleDelete,
  };
}

function EnabledPill() {
  const { enabled, setEnabled } = useLinearEnabled();
  return <DraftedIntegrationEnabledControl id="linear" enabled={enabled} persist={setEnabled} />;
}

export function LinearConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const s = useLinearSettings(workspaceId);
  const baseline = configToForm(s.baselineConfig);
  const missingSecret = !s.config?.hasSecret && !s.form.secret;
  const disableSave = s.saving || missingSecret;
  const disableTest = missingSecret;
  const revision = JSON.stringify(s.form);
  const dirty = !s.loading && revision !== JSON.stringify(configToForm(s.baselineConfig));

  useSettingsSaveContributor({
    id: `linear-config:${workspaceId}`,
    revision,
    isDirty: dirty,
    canSave: !disableSave,
    invalidReason: missingSecret ? t("linear:anApiKeyIsRequired") : undefined,
    save: s.handleSave,
    discard: s.discard,
  });

  return (
    <SettingsSection
      icon={<IconHexagon className="h-5 w-5" />}
      title={t("linear:linearIntegration")}
      description={t("linear:linearIntegrationDescription")}
      action={<EnabledPill />}
    >
      <SettingsCard isDirty={dirty}>
        <CardContent className="space-y-4 pt-6">
          <IntegrationAuthStatusBanner health={s.health} />
          <SecretField
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            update={s.update}
            hasSavedSecret={!!s.config?.hasSecret}
          />
          <TeamSelector
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            update={s.update}
            hasSavedSecret={!!s.config?.hasSecret}
            teams={s.teams}
            loadingTeams={s.loadingTeams}
          />
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

type LinearIntegrationPageProps = {
  workspaceId?: string;
};

export function LinearIntegrationPage({ workspaceId }: LinearIntegrationPageProps = {}) {
  return (
    <div className="space-y-8">
      <WorkspaceScopedSection workspaceId={workspaceId}>
        {(workspaceId) => <LinearConnectionSection key={workspaceId} workspaceId={workspaceId} />}
      </WorkspaceScopedSection>
      <LinearIssueWatchersSection />
    </div>
  );
}
