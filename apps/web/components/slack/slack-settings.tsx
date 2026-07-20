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
import Link from "@/components/routing/app-link";
import { IconBrandSlack } from "@tabler/icons-react";
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
import { useSlackEnabled } from "@/hooks/domains/slack/use-slack-enabled";
import {
  IntegrationAuthStatusBanner,
  type IntegrationAuthHealth,
} from "@/components/integrations/auth-status-banner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import {
  setSlackConfig,
  deleteSlackConfig,
  testSlackConnection,
} from "@/lib/api/domains/slack-api";
import { listUtilityAgents, type UtilityAgent } from "@/lib/api/domains/utility-api";
import { qk } from "@/lib/query/keys";
import { slackConfigQueryOptions } from "@/lib/query/query-options/slack";
import type { SlackConfig, TestSlackConnectionResult } from "@/lib/types/slack";
import {
  DEFAULT_POLL_INTERVAL_SECONDS,
  DEFAULT_PREFIX,
  MAX_POLL_INTERVAL_SECONDS,
  MIN_POLL_INTERVAL_SECONDS,
  emptySlackSettingsForm,
  slackConfigToForm,
  type SlackSettingsFormState as FormState,
} from "./slack-settings-form";

function configToHealth(config: SlackConfig | null): IntegrationAuthHealth | null {
  if (!config?.hasToken || !config.hasCookie) return null;
  if (!config.lastCheckedAt) return { ok: false, error: "", checkedAt: null };
  return {
    ok: !!config.lastOk,
    error: config.lastError ?? "",
    checkedAt: new Date(config.lastCheckedAt),
  };
}

type SecretFieldsProps = {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  hasSavedToken: boolean;
  hasSavedCookie: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

function SecretFields({
  form,
  baseline,
  loading,
  hasSavedToken,
  hasSavedCookie,
  update,
}: SecretFieldsProps) {
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="slack-token">
          Session token (xoxc-…)
          {hasSavedToken && (
            <span className="text-xs text-muted-foreground ml-2">
              (saved — leave blank to keep)
            </span>
          )}
        </Label>
        <Input
          id="slack-token"
          type="password"
          placeholder={hasSavedToken ? "••••••••" : "xoxc-..."}
          value={form.token}
          data-settings-dirty={form.token !== baseline.token}
          onChange={(e) => update("token", e.target.value)}
          disabled={loading}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="slack-cookie">
          d cookie value
          {hasSavedCookie && (
            <span className="text-xs text-muted-foreground ml-2">
              (saved — leave blank to keep)
            </span>
          )}
        </Label>
        <Input
          id="slack-cookie"
          type="password"
          placeholder={hasSavedCookie ? "••••••••" : "xoxd-..."}
          value={form.cookie}
          data-settings-dirty={form.cookie !== baseline.cookie}
          onChange={(e) => update("cookie", e.target.value)}
          disabled={loading}
        />
        <p className="text-xs text-muted-foreground">
          Open Slack in your browser, copy the value of the `d` cookie and the `xoxc-` token from
          local storage. Both are required.
        </p>
      </div>
    </div>
  );
}

type UtilityAgentPickerProps = {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  agents: UtilityAgent[];
  loadingAgents: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

function utilityAgentPlaceholder(agents: UtilityAgent[], loading: boolean): string {
  if (loading) return "Loading…";
  if (agents.length === 0) return "Create one in Settings → Utility agents";
  return "Choose a utility agent";
}

function isAgentSelectable(a: UtilityAgent): boolean {
  if (a.builtin) return true;
  return a.enabled && !!a.agent_id && !!a.model;
}

function modelSuffix(a: UtilityAgent): string {
  if (a.model) return ` (${a.model})`;
  if (a.builtin) return " (uses default model)";
  return "";
}

function utilityAgentLabel(a: UtilityAgent): string {
  const base = `${a.name}${modelSuffix(a)}`;
  if (a.builtin) return base;
  if (!a.enabled) return `${base} — disabled`;
  if (!a.agent_id || !a.model) return `${base} — not configured`;
  return base;
}

function UtilityAgentPicker({
  form,
  baseline,
  loading,
  agents,
  loadingAgents,
  update,
}: UtilityAgentPickerProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="slack-utility-agent">Triage agent</Label>
      <Select
        value={form.utilityAgentId || ""}
        onValueChange={(v) => update("utilityAgentId", v)}
        disabled={loading || loadingAgents || agents.length === 0}
      >
        <SelectTrigger
          id="slack-utility-agent"
          className="w-full"
          data-settings-dirty={form.utilityAgentId !== baseline.utilityAgentId}
        >
          <SelectValue placeholder={utilityAgentPlaceholder(agents, loadingAgents)} />
        </SelectTrigger>
        <SelectContent>
          {agents.map((a) => (
            <SelectItem key={a.id} value={a.id} disabled={!isAgentSelectable(a)}>
              {utilityAgentLabel(a)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">
        The utility agent that interprets each Slack message and creates the Kandev task. It runs
        with Kandev MCP tools wired in (list_workspaces_kandev, create_task_kandev, …) so it picks
        the destination Kandev workspace + workflow + repo from context. Built-in agents use your
        default model from{" "}
        <Link href="/settings/utility-agents" className="underline cursor-pointer">
          Settings → Utility agents
        </Link>
        .
      </p>
      <p className="text-xs text-muted-foreground">
        Custom prompts can reference <code>{"{{SlackInstruction}}"}</code>,{" "}
        <code>{"{{SlackThread}}"}</code>, <code>{"{{SlackPermalink}}"}</code>,{" "}
        <code>{"{{SlackUser}}"}</code>, <code>{"{{SlackChannelID}}"}</code>, and{" "}
        <code>{"{{SlackTS}}"}</code>. When at least one is used, your template owns the full prompt;
        otherwise the default Slack-triage system prompt is prepended automatically.
      </p>
    </div>
  );
}

function PrefixField({
  form,
  baseline,
  loading,
  update,
}: {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="slack-prefix">Command prefix</Label>
      <Input
        id="slack-prefix"
        type="text"
        placeholder={DEFAULT_PREFIX}
        value={form.commandPrefix}
        data-settings-dirty={form.commandPrefix !== baseline.commandPrefix}
        onChange={(e) => update("commandPrefix", e.target.value)}
        disabled={loading}
      />
      <p className="text-xs text-muted-foreground">
        Messages you write in Slack starting with this prefix become Kandev tasks. Default:{" "}
        <code>{DEFAULT_PREFIX} &lt;instruction&gt;</code>.
      </p>
    </div>
  );
}

function PollIntervalField({
  form,
  baseline,
  loading,
  update,
}: {
  form: FormState;
  baseline: FormState;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor="slack-poll-interval">Polling interval (seconds)</Label>
      <Input
        id="slack-poll-interval"
        type="number"
        min={MIN_POLL_INTERVAL_SECONDS}
        max={MAX_POLL_INTERVAL_SECONDS}
        step={1}
        value={form.pollIntervalSeconds}
        data-settings-dirty={form.pollIntervalSeconds !== baseline.pollIntervalSeconds}
        onChange={(e) => {
          const n = Number(e.target.value);
          update("pollIntervalSeconds", Number.isFinite(n) ? n : DEFAULT_POLL_INTERVAL_SECONDS);
        }}
        disabled={loading}
      />
      <p className="text-xs text-muted-foreground">
        How often Slack is checked for new <code>{form.commandPrefix || DEFAULT_PREFIX}</code>{" "}
        messages. Lower = more responsive, higher = fewer Slack API calls. Range:{" "}
        {MIN_POLL_INTERVAL_SECONDS}–{MAX_POLL_INTERVAL_SECONDS}s. Default:{" "}
        {DEFAULT_POLL_INTERVAL_SECONDS}s.
      </p>
    </div>
  );
}

function TestResultAlert({ result }: { result: TestSlackConnectionResult | null }) {
  if (!result) return null;
  const teamSuffix = result.teamName ? ` (${result.teamName})` : "";
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {result.ok
          ? `Connected as ${result.displayName || result.userId}${teamSuffix}`
          : `Failed: ${result.error}`}
      </AlertDescription>
    </Alert>
  );
}

function UnsupportedWarning() {
  return (
    <Alert>
      <AlertDescription className="text-xs">
        <strong>Browser session auth (unsupported):</strong> Slack rotates session cookies often, so
        you may need to reconnect when authentication expires. Bot installs and user OAuth are on
        the roadmap.
      </AlertDescription>
    </Alert>
  );
}

type ActionBarProps = {
  testing: boolean;
  deleting: boolean;
  loading: boolean;
  hasConfig: boolean;
  disableTest: boolean;
  onTest: () => void;
  onDelete: () => void;
};

function ActionBar({
  testing,
  deleting,
  loading,
  hasConfig,
  disableTest,
  onTest,
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
        title={disableTest ? "Paste a token and cookie to test the connection" : undefined}
      >
        {testing ? "Testing..." : "Test connection"}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          disabled={deleting}
          className="ml-auto cursor-pointer"
        >
          {deleting ? "Removing..." : "Remove configuration"}
        </Button>
      )}
    </div>
  );
}

function useUtilityAgentsLoader() {
  const [agents, setAgents] = useState<UtilityAgent[] | null>(null);
  useEffect(() => {
    let cancelled = false;
    listUtilityAgents({ cache: "no-store" })
      .then((res) => {
        if (!cancelled) setAgents(res.agents ?? []);
      })
      .catch(() => {
        if (!cancelled) setAgents([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return { agents: agents ?? [], loadingAgents: agents === null };
}

type SettingsActionsArgs = {
  workspaceId: string;
  form: FormState;
  setConfig: (cfg: SlackConfig | null) => void;
  setForm: Dispatch<SetStateAction<FormState>>;
  setTestResult: (r: TestSlackConnectionResult | null) => void;
};

function useSettingsActions({
  workspaceId,
  form,
  setConfig,
  setForm,
  setTestResult,
}: SettingsActionsArgs) {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await testSlackConnection(
        {
          authMethod: "cookie",
          utilityAgentId: form.utilityAgentId,
          commandPrefix: form.commandPrefix,
          pollIntervalSeconds: form.pollIntervalSeconds,
          token: form.token || undefined,
          cookie: form.cookie || undefined,
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
      const saved = await setSlackConfig(
        {
          authMethod: "cookie",
          utilityAgentId: form.utilityAgentId,
          commandPrefix: form.commandPrefix,
          pollIntervalSeconds: form.pollIntervalSeconds,
          token: form.token || undefined,
          cookie: form.cookie || undefined,
        },
        { workspaceId },
      );
      queryClient.setQueryData(qk.integrations.slack.config(workspaceId), saved);
      setConfig(saved);
      setForm((current) =>
        JSON.stringify(current) === JSON.stringify(submitted) ? slackConfigToForm(saved) : current,
      );
      setTestResult(null);
      toast({ description: "Slack configuration saved", variant: "success" });
    } catch (err) {
      toast({ description: `Save failed: ${String(err)}`, variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, form, queryClient, toast, setConfig, setForm, setTestResult]);

  const handleDelete = useCallback(async () => {
    if (!confirm("Remove Slack configuration?")) return;
    setDeleting(true);
    try {
      await deleteSlackConfig({ workspaceId });
      queryClient.setQueryData(qk.integrations.slack.config(workspaceId), null);
      setConfig(null);
      setForm(emptySlackSettingsForm);
      setTestResult(null);
      toast({ description: "Slack configuration removed", variant: "success" });
    } catch (err) {
      toast({ description: `Delete failed: ${String(err)}`, variant: "error" });
    } finally {
      setDeleting(false);
    }
  }, [workspaceId, queryClient, toast, setConfig, setForm, setTestResult]);

  return { saving, testing, deleting, handleTest, handleSave, handleDelete };
}

function useSlackSettings(workspaceId: string) {
  const { toast } = useToast();
  const configQuery = useQuery(slackConfigQueryOptions(workspaceId));
  const [config, setConfig] = useState<SlackConfig | null>(null);
  const [form, setForm] = useState<FormState>(emptySlackSettingsForm);
  const [testResult, setTestResult] = useState<TestSlackConnectionResult | null>(null);
  const formHydratedRef = useRef(false);
  const health = configToHealth(config);
  const { agents, loadingAgents } = useUtilityAgentsLoader();

  useEffect(() => {
    if (!configQuery.isSuccess) return;
    const cfg = configQuery.data ?? null;
    setConfig(cfg);
    if (!formHydratedRef.current) {
      setForm(slackConfigToForm(cfg));
      formHydratedRef.current = true;
    }
  }, [configQuery.data, configQuery.isSuccess]);

  useEffect(() => {
    if (!configQuery.isError) return;
    toast({
      description: `Failed to load Slack config: ${String(configQuery.error)}`,
      variant: "error",
    });
  }, [configQuery.error, configQuery.isError, toast]);

  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );
  const discard = useCallback(() => setForm(slackConfigToForm(config)), [config]);

  const { saving, testing, deleting, handleTest, handleSave, handleDelete } = useSettingsActions({
    workspaceId,
    form,
    setConfig,
    setForm,
    setTestResult,
  });

  return {
    config,
    form,
    loading: configQuery.isFetching && !configQuery.isSuccess,
    saving,
    testing,
    deleting,
    testResult,
    health,
    agents,
    loadingAgents,
    update,
    discard,
    handleTest,
    handleSave,
    handleDelete,
  };
}

function EnabledPill() {
  const { enabled, setEnabled } = useSlackEnabled();
  return <DraftedIntegrationEnabledControl id="slack" enabled={enabled} persist={setEnabled} />;
}

export function SlackConnectionSection({ workspaceId }: { workspaceId: string }) {
  const s = useSlackSettings(workspaceId);
  const baseline = slackConfigToForm(s.config);
  const missingSecrets =
    (!s.config?.hasToken && !s.form.token) || (!s.config?.hasCookie && !s.form.cookie);
  const missingAgent = !s.form.utilityAgentId;
  const disableSave = s.saving || missingSecrets || missingAgent;
  const disableTest = missingSecrets;
  const revision = JSON.stringify(s.form);
  const dirty = !s.loading && revision !== JSON.stringify(slackConfigToForm(s.config));
  let invalidReason: string | undefined;
  if (missingSecrets) invalidReason = "A session token and cookie are required.";
  else if (missingAgent) invalidReason = "A triage agent is required.";

  useSettingsSaveContributor({
    id: `slack-config:${workspaceId}`,
    revision,
    isDirty: dirty,
    canSave: !disableSave,
    invalidReason,
    save: s.handleSave,
    discard: s.discard,
  });

  return (
    <SettingsSection
      icon={<IconBrandSlack className="h-5 w-5" />}
      title="Slack integration"
      description="Capture Slack threads as tasks for the selected workspace. Type !kandev <instruction> in any thread you can see and the configured utility agent creates the task."
      action={<EnabledPill />}
    >
      <SettingsCard isDirty={dirty}>
        <CardContent className="space-y-4 pt-6">
          <UnsupportedWarning />
          <IntegrationAuthStatusBanner health={s.health} />
          <SecretFields
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            hasSavedToken={!!s.config?.hasToken}
            hasSavedCookie={!!s.config?.hasCookie}
            update={s.update}
          />
          <Separator />
          <UtilityAgentPicker
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            agents={s.agents}
            loadingAgents={s.loadingAgents}
            update={s.update}
          />
          <PrefixField form={s.form} baseline={baseline} loading={s.loading} update={s.update} />
          <PollIntervalField
            form={s.form}
            baseline={baseline}
            loading={s.loading}
            update={s.update}
          />
          <TestResultAlert result={s.testResult} />
          <Separator />
          <ActionBar
            testing={s.testing}
            deleting={s.deleting}
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

type SlackIntegrationPageProps = {
  workspaceId?: string;
};

export function SlackIntegrationPage({ workspaceId }: SlackIntegrationPageProps = {}) {
  return (
    <div className="space-y-8">
      <WorkspaceScopedSection workspaceId={workspaceId}>
        {(workspaceId) => <SlackConnectionSection key={workspaceId} workspaceId={workspaceId} />}
      </WorkspaceScopedSection>
    </div>
  );
}
