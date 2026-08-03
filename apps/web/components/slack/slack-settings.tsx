"use client";

import { Trans, useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { IconBrandSlack } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { useSlackEnabled } from "@/hooks/domains/slack/use-slack-enabled";
import { IntegrationAuthStatusBanner } from "@/components/integrations/auth-status-banner";
import { WorkspaceScopedSection } from "@/components/integrations/workspace-scoped-section";
import { DraftedIntegrationEnabledControl } from "@/components/integrations/drafted-integration-enabled-control";
import { type UtilityAgent } from "@/lib/api/domains/utility-api";
import type { TestSlackConnectionResult } from "@/lib/types/slack";
import {
  DEFAULT_PREFIX,
  DEFAULT_PREFIX_EXAMPLE,
  DEFAULT_POLL_INTERVAL_SECONDS,
  MAX_POLL_INTERVAL_SECONDS,
  MIN_POLL_INTERVAL_SECONDS,
  PROMPT_TOKENS,
  SECRET_MASK,
  SLACK_COOKIE_NAME,
  SLACK_COOKIE_PLACEHOLDER,
  SLACK_TOKEN_PLACEHOLDER,
  SLACK_TOKEN_PREFIX,
  UTILITY_AGENTS_ROUTE,
  configToForm,
  useSlackSettings,
  type FormState,
} from "./slack-settings-state";

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
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <Label htmlFor="slack-token">
          {t("slack:sessionToken", { prefix: SLACK_TOKEN_PREFIX })}
          {hasSavedToken && (
            <span className="text-xs text-muted-foreground ml-2">{t("slack:savedLeaveBlank")}</span>
          )}
        </Label>
        <Input
          id="slack-token"
          type="password"
          placeholder={hasSavedToken ? SECRET_MASK : SLACK_TOKEN_PLACEHOLDER}
          value={form.token}
          data-settings-dirty={form.token !== baseline.token}
          onChange={(e) => update("token", e.target.value)}
          disabled={loading}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="slack-cookie">
          {t("slack:cookieValue", { cookie: SLACK_COOKIE_NAME })}
          {hasSavedCookie && (
            <span className="text-xs text-muted-foreground ml-2">{t("slack:savedLeaveBlank")}</span>
          )}
        </Label>
        <Input
          id="slack-cookie"
          type="password"
          placeholder={hasSavedCookie ? SECRET_MASK : SLACK_COOKIE_PLACEHOLDER}
          value={form.cookie}
          data-settings-dirty={form.cookie !== baseline.cookie}
          onChange={(e) => update("cookie", e.target.value)}
          disabled={loading}
        />
        <p className="text-xs text-muted-foreground">
          {t("slack:credentialsHelp", {
            cookie: SLACK_COOKIE_NAME,
            prefix: SLACK_TOKEN_PREFIX,
          })}
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

type Translate = (key: string, values?: Record<string, unknown>) => string;

function utilityAgentPlaceholder(t: Translate, agents: UtilityAgent[], loading: boolean): string {
  if (loading) return t("slack:loadingEllipsis");
  if (agents.length === 0) return t("slack:createUtilityAgentFirst");
  return t("slack:chooseUtilityAgent");
}

function isAgentSelectable(a: UtilityAgent): boolean {
  if (a.builtin) return true;
  return a.enabled && !!a.agent_id && !!a.model;
}

// `name` and `model` are the agent record — data, so the label is assembled from
// catalog messages that interpolate them rather than by concatenating copy.
function utilityAgentBaseLabel(t: Translate, a: UtilityAgent): string {
  if (a.model) return t("slack:agentWithModel", { name: a.name, model: a.model });
  if (a.builtin) return t("slack:agentUsesDefaultModel", { name: a.name });
  return a.name;
}

function utilityAgentLabel(t: Translate, a: UtilityAgent): string {
  const base = utilityAgentBaseLabel(t, a);
  if (a.builtin) return base;
  if (!a.enabled) return t("slack:agentDisabled", { agent: base });
  if (!a.agent_id || !a.model) return t("slack:agentNotConfigured", { agent: base });
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
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label htmlFor="slack-utility-agent">{t("slack:triageAgent")}</Label>
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
          <SelectValue placeholder={utilityAgentPlaceholder(t, agents, loadingAgents)} />
        </SelectTrigger>
        <SelectContent>
          {agents.map((a) => (
            <SelectItem key={a.id} value={a.id} disabled={!isAgentSelectable(a)}>
              {utilityAgentLabel(t, a)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">
        {/* The three MCP tool names are identifiers, so they are interpolated
            rather than written into the catalog. */}
        <Trans
          i18nKey="slack:triageAgentHelp"
          values={{ tools: "list_workspaces_kandev, create_task_kandev, …" }}
        >
          The utility agent that interprets each Slack message and creates the Kandev task. It runs
          with Kandev MCP tools wired in ({"{{tools}}"}) so it picks the destination Kandev
          workspace + workflow + repo from context. Built-in agents use your default model from{" "}
          <Link href={UTILITY_AGENTS_ROUTE} className="underline cursor-pointer">
            Settings → Utility agents
          </Link>
          .
        </Trans>
      </p>
      <p className="text-xs text-muted-foreground">
        {/* The six tokens are prompt syntax the backend substitutes, so they are
            interpolated as one monospace run rather than written into the
            catalog — six separate <code> children would make the tag indices
            drift on any reflow. */}
        <Trans i18nKey="slack:promptTokensHelp" values={{ tokens: PROMPT_TOKENS.join(", ") }}>
          Custom prompts can reference <code>{"{{tokens}}"}</code>. When at least one is used, your
          template owns the full prompt; otherwise the default Slack-triage system prompt is
          prepended automatically.
        </Trans>
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
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label htmlFor="slack-prefix">{t("slack:commandPrefix")}</Label>
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
        {/* Not a <Trans>: the example contains literal angle brackets, which
            html-parse-stringify would read as a tag inside the message. */}
        {t("slack:commandPrefixHelp")} <code>{DEFAULT_PREFIX_EXAMPLE}</code>.
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
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label htmlFor="slack-poll-interval">{t("slack:pollingInterval")}</Label>
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
        <Trans
          i18nKey="slack:pollingIntervalHelp"
          values={{
            prefix: form.commandPrefix || DEFAULT_PREFIX,
            min: MIN_POLL_INTERVAL_SECONDS,
            max: MAX_POLL_INTERVAL_SECONDS,
            fallback: DEFAULT_POLL_INTERVAL_SECONDS,
          }}
        >
          How often Slack is checked for new <code>{"{{prefix}}"}</code> messages. Lower = more
          responsive, higher = fewer Slack API calls. Range: {"{{min}}"}–{"{{max}}"}s. Default:{" "}
          {"{{fallback}}"}s.
        </Trans>
      </p>
    </div>
  );
}

// `displayName` / `userId` / `teamName` all come from the Slack API — data, so
// they are interpolated into the message rather than concatenated around it.
function testResultMessage(t: Translate, result: TestSlackConnectionResult): string {
  if (!result.ok) return t("slack:testFailed", { error: result.error });
  const name = result.displayName || result.userId;
  return result.teamName
    ? t("slack:connectedAsWithTeam", { name, team: result.teamName })
    : t("slack:connectedAs", { name });
}

function TestResultAlert({ result }: { result: TestSlackConnectionResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>{testResultMessage(t, result)}</AlertDescription>
    </Alert>
  );
}

function UnsupportedWarning() {
  return (
    <Alert>
      <AlertDescription className="text-xs">
        <Trans i18nKey="slack:unsupportedAuthWarning">
          <strong>Browser session auth (unsupported):</strong> Slack rotates session cookies often,
          so you may need to reconnect when authentication expires. Bot installs and user OAuth are
          on the roadmap.
        </Trans>
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
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={testing || loading || disableTest}
        className="cursor-pointer"
        title={disableTest ? t("slack:pasteCredentialsToTest") : undefined}
      >
        {testing ? t("slack:testing") : t("slack:testConnection")}
      </Button>
      {hasConfig && (
        <Button
          type="button"
          variant="destructive"
          onClick={onDelete}
          disabled={deleting}
          className="ml-auto cursor-pointer"
        >
          {deleting ? t("slack:removing") : t("slack:removeConfiguration")}
        </Button>
      )}
    </div>
  );
}

function EnabledPill() {
  const { enabled, setEnabled } = useSlackEnabled();
  return <DraftedIntegrationEnabledControl id="slack" enabled={enabled} persist={setEnabled} />;
}

export function SlackConnectionSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const s = useSlackSettings(workspaceId);
  const baseline = configToForm(s.config);
  const missingSecrets =
    (!s.config?.hasToken && !s.form.token) || (!s.config?.hasCookie && !s.form.cookie);
  const missingAgent = !s.form.utilityAgentId;
  const disableSave = s.saving || missingSecrets || missingAgent;
  const disableTest = missingSecrets;
  const revision = JSON.stringify(s.form);
  const dirty = !s.loading && revision !== JSON.stringify(configToForm(s.config));
  let invalidReason: string | undefined;
  if (missingSecrets) invalidReason = t("slack:secretsRequired");
  else if (missingAgent) invalidReason = t("slack:triageAgentRequired");

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
      title={t("slack:integrationTitle")}
      description={t("slack:integrationDescription", { example: DEFAULT_PREFIX_EXAMPLE })}
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
