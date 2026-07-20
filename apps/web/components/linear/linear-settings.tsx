"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
import {
  setLinearConfig,
  deleteLinearConfig,
  testLinearConnection,
  listLinearTeams,
} from "@/lib/api/domains/linear-api";
import { qk } from "@/lib/query/keys";
import { linearConfigQueryOptions } from "@/lib/query/query-options/linear";
import type { LinearConfig, LinearTeam, TestLinearConnectionResult } from "@/lib/types/linear";
import { LinearIssueWatchersSection } from "./linear-issue-watchers-section";

type FormState = {
  defaultTeamKey: string;
  secret: string;
};

const emptyForm: FormState = { defaultTeamKey: "", secret: "" };

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
  return (
    <div className="space-y-1.5">
      <Label htmlFor="linear-secret">
        API key
        {hasSavedSecret && (
          <span className="text-xs text-muted-foreground ml-2">
            (saved — leave blank to keep the current value)
          </span>
        )}
      </Label>
      <Input
        id="linear-secret"
        data-testid="linear-secret-input"
        type="password"
        placeholder={hasSavedSecret ? "••••••••" : "lin_api_..."}
        value={form.secret}
        data-settings-dirty={form.secret !== baseline.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
      <p className="text-xs text-muted-foreground">
        Create a personal API key at{" "}
        <a
          className="underline cursor-pointer"
          href="https://linear.app/settings/account/security"
          target="_blank"
          rel="noreferrer"
        >
          linear.app/settings/account/security
        </a>
      </p>
    </div>
  );
}

function TeamSelector({ form, baseline, loading, update, teams, loadingTeams }: FieldsRowProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="linear-team">Default team (optional)</Label>
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
          <SelectValue placeholder={loadingTeams ? "Loading teams…" : "Choose a team"} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__none__">No default</SelectItem>
          {teams.map((t) => (
            <SelectItem key={t.id} value={t.key}>
              {t.name} ({t.key})
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function TestResultAlert({ result }: { result: TestLinearConnectionResult | null }) {
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {result.ok
          ? `Connected as ${result.displayName || result.email || result.userId}${result.orgName ? ` (${result.orgName})` : ""}`
          : `Failed: ${result.error}`}
      </AlertDescription>
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
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest}
        className="cursor-pointer"
        title={disableTest ? "Paste an API key to test the connection" : undefined}
        data-testid="linear-test-button"
      >
        {testing ? "Testing..." : "Test connection"}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          className="ml-auto cursor-pointer"
          data-testid="linear-delete-button"
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
  const { toast } = useToast();
  const queryClient = useQueryClient();
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
      queryClient.setQueryData(qk.integrations.linear.config(workspaceId), saved);
      setConfig(saved);
      setBaselineConfig(saved);
      setForm((current) =>
        JSON.stringify(current) === JSON.stringify(submitted) ? configToForm(saved) : current,
      );
      setTestResult(null);
      toast({ description: "Linear configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, queryClient, toast, setConfig, setBaselineConfig, setForm, setTestResult]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Linear configuration?")) return;
    try {
      await deleteLinearConfig({ workspaceId });
      queryClient.setQueryData(qk.integrations.linear.config(workspaceId), null);
      setConfig(null);
      setBaselineConfig(null);
      setForm(emptyForm);
      setTestResult(null);
      toast({ description: "Linear configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    }
  }, [workspaceId, queryClient, toast, setConfig, setBaselineConfig, setForm, setTestResult]);

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
  const { toast } = useToast();
  const configQuery = useQuery(linearConfigQueryOptions(workspaceId));
  const [config, setConfig] = useState<LinearConfig | null>(null);
  const [baselineConfig, setBaselineConfig] = useState<LinearConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [testResult, setTestResult] = useState<TestLinearConnectionResult | null>(null);
  const formHydratedRef = useRef(false);
  const health = configToHealth(config);
  const { teams, loadingTeams } = useTeamsLoader(workspaceId, config?.hasSecret, config?.lastOk);

  useEffect(() => {
    if (!configQuery.isSuccess) return;
    const cfg = configQuery.data ?? null;
    setConfig(cfg);
    if (!formHydratedRef.current) {
      setBaselineConfig(cfg);
      setForm(configToForm(cfg));
      formHydratedRef.current = true;
    }
  }, [configQuery.data, configQuery.isSuccess]);

  useEffect(() => {
    if (!configQuery.isError) return;
    toast({
      description: `Failed to load Linear config: ${String(configQuery.error)}`,
      variant: "error",
    });
  }, [configQuery.error, configQuery.isError, toast]);

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
    loading: configQuery.isFetching && !configQuery.isSuccess,
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
    invalidReason: missingSecret ? "An API key is required." : undefined,
    save: s.handleSave,
    discard: s.discard,
  });

  return (
    <SettingsSection
      icon={<IconHexagon className="h-5 w-5" />}
      title="Linear integration"
      description="Connect this workspace to Linear with a personal API key. Credentials are stored encrypted server-side for the selected workspace."
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
