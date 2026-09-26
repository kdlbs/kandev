"use client";

import { Trans, useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import type { ReactNode } from "react";
import { Badge } from "@kandev/ui/badge";
import { Checkbox } from "@kandev/ui/checkbox";
import { AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { AgentLogo } from "@/components/agent-logo";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import type { RemoteAuthMethod, RemoteAuthSpec } from "@/lib/api/domains/settings-api";
import type { AgentConfigBundle } from "@/lib/api/domains/agent-config-api";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { AgentConfigOptions } from "./portable-config-bundles";

const OPTION_LABEL_BASE =
  "flex min-h-11 w-full cursor-pointer items-start gap-3 rounded-md border p-3 text-left transition-colors";
const SELECTED_BORDER = "border-primary bg-primary/5";
const DEFAULT_BORDER = "border-border";
const AGENT_LOGO_IDS = new Set(["claude_code", "auggie", "codex", "gemini", "copilot", "amp"]);

export function getSpecMethods(spec: RemoteAuthSpec): RemoteAuthMethod[] {
  return Array.isArray(spec.methods) ? spec.methods : [];
}

type AuthSectionProps = {
  spec: RemoteAuthSpec;
  selectedIds: Set<string>;
  baselineSelectedIds: Set<string>;
  onCredentialsChange: (ids: string[]) => void;
  agentEnvVars: Record<string, string | null>;
  baselineAgentEnvVars: Record<string, string | null>;
  onMethodSecretChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
  configBundles: AgentConfigBundle[];
  configBundleIds: string[];
  baselineConfigBundleIds: string[];
  onConfigBundleChange: (ids: string[]) => void;
  isSSH: boolean;
};

export function AuthSection({
  spec,
  selectedIds,
  baselineSelectedIds,
  onCredentialsChange,
  agentEnvVars,
  baselineAgentEnvVars,
  onMethodSecretChange,
  secrets,
  configBundles,
  configBundleIds,
  baselineConfigBundleIds,
  onConfigBundleChange,
  isSSH,
}: AuthSectionProps) {
  const methods = getSpecMethods(spec);
  const envMethods = methods.filter((m) => m.type === "env");
  const envMethodIds = new Set(envMethods.map((m) => m.method_id));
  const fileMethod = methods.find((m) => m.type === "files");
  const fileSelected = fileMethod ? selectedIds.has(fileMethod.method_id) : false;
  const baselineFileSelected = fileMethod ? baselineSelectedIds.has(fileMethod.method_id) : false;
  const isEnvSelected = (
    m: RemoteAuthMethod,
    ids: Set<string>,
    envVars: Record<string, string | null>,
  ) => ids.has(m.method_id) || Boolean(envVars[m.method_id]);
  const anyEnvSelected = envMethods.some((m) => isEnvSelected(m, selectedIds, agentEnvVars));
  const anyEnvHasSecret = envMethods.some((m) => Boolean(agentEnvVars[m.method_id]));
  const envDirty = envMethods.some(
    (m) =>
      isEnvSelected(m, selectedIds, agentEnvVars) !==
        isEnvSelected(m, baselineSelectedIds, baselineAgentEnvVars) ||
      (agentEnvVars[m.method_id] ?? null) !== (baselineAgentEnvVars[m.method_id] ?? null),
  );
  const isDirty = fileSelected !== baselineFileSelected || envDirty;
  const configIsDirty = configBundles.some(
    (bundle) => configBundleIds.includes(bundle.id) !== baselineConfigBundleIds.includes(bundle.id),
  );

  const handleMethodToggle = (methodId: string, checked: boolean) => {
    const nextSelectedIds = new Set(selectedIds);
    setMethodSelected(nextSelectedIds, methodId, checked);
    if (envMethodIds.has(methodId) && !checked) {
      onMethodSecretChange(methodId, null);
    }
    onCredentialsChange([...nextSelectedIds]);
  };

  return (
    <AccordionItem value={spec.id} data-settings-dirty={isDirty || configIsDirty}>
      <AccordionTrigger>
        <div className="flex items-center gap-2 flex-1">
          {AGENT_LOGO_IDS.has(spec.id) && <AgentLogo agentName={spec.id} size={18} />}
          <span className="font-medium text-sm">{spec.display_name}</span>
          <AuthStatusBadge
            fileSelected={fileSelected}
            envSelected={anyEnvSelected}
            hasSecret={anyEnvHasSecret}
          />
        </div>
      </AccordionTrigger>
      <AccordionContent className="h-auto">
        <div className="space-y-3 text-sm">
          <AuthOptions
            fileMethod={fileMethod}
            envMethods={envMethods}
            fileSelected={fileSelected}
            baselineFileSelected={baselineFileSelected}
            agentEnvVars={agentEnvVars}
            baselineAgentEnvVars={baselineAgentEnvVars}
            selectedIds={selectedIds}
            baselineSelectedIds={baselineSelectedIds}
            onMethodToggle={handleMethodToggle}
            onSecretIdChange={(methodId, sid) => onMethodSecretChange(methodId, sid)}
            secrets={secrets}
          />
          {configBundles.length > 0 && (
            <AgentConfigOptions
              agentId={spec.id}
              bundles={configBundles}
              selectedIds={configBundleIds}
              baselineSelectedIds={baselineConfigBundleIds}
              onChange={onConfigBundleChange}
              isSSH={isSSH}
            />
          )}
        </div>
      </AccordionContent>
    </AccordionItem>
  );
}

function AuthOptions({
  fileMethod,
  envMethods,
  fileSelected,
  baselineFileSelected,
  agentEnvVars,
  baselineAgentEnvVars,
  selectedIds,
  baselineSelectedIds,
  onMethodToggle,
  onSecretIdChange,
  secrets,
}: {
  fileMethod?: RemoteAuthMethod;
  envMethods: RemoteAuthMethod[];
  fileSelected: boolean;
  baselineFileSelected: boolean;
  agentEnvVars: Record<string, string | null>;
  baselineAgentEnvVars: Record<string, string | null>;
  selectedIds: Set<string>;
  baselineSelectedIds: Set<string>;
  onMethodToggle: (methodId: string, checked: boolean) => void;
  onSecretIdChange: (methodId: string, id: string | null) => void;
  secrets: SecretListItem[];
}) {
  const { t } = useTranslation();
  const showEnvVarInLabel = envMethods.length > 1;
  return (
    <div role="group" aria-label={t("executors:remoteAuthMethod")} className="grid gap-2">
      {fileMethod && (
        <FileOption
          method={fileMethod}
          isSelected={fileSelected}
          isDirty={fileSelected !== baselineFileSelected}
          filesAvailable={fileMethod.has_local_files ?? false}
          onCheckedChange={(checked) => onMethodToggle(fileMethod.method_id, checked)}
        />
      )}
      {envMethods
        .filter((method) => method.env_var)
        .map((method) => {
          const secretId = agentEnvVars[method.method_id] ?? null;
          const baselineSecretId = baselineAgentEnvVars[method.method_id] ?? null;
          const isSelected = selectedIds.has(method.method_id) || Boolean(secretId);
          const baselineSelected =
            baselineSelectedIds.has(method.method_id) || Boolean(baselineSecretId);
          return (
            <EnvOption
              key={method.method_id}
              method={method}
              isSelected={isSelected}
              isDirty={isSelected !== baselineSelected || secretId !== baselineSecretId}
              secretId={secretId}
              baselineSecretId={baselineSecretId}
              showEnvVarInLabel={showEnvVarInLabel}
              onSecretIdChange={(sid) => onSecretIdChange(method.method_id, sid)}
              secrets={secrets}
              onCheckedChange={(checked) => onMethodToggle(method.method_id, checked)}
            />
          );
        })}
    </div>
  );
}

function FileOption({
  method,
  isSelected,
  isDirty,
  filesAvailable,
  onCheckedChange,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  isDirty: boolean;
  filesAvailable: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const filesLabel = method.source_files?.join(", ") ?? "";
  return (
    <AuthOption
      selected={isSelected}
      isDirty={isDirty}
      onCheckedChange={onCheckedChange}
      label={method.label ?? t("executors:copyAuthFiles")}
    >
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{method.label ?? t("executors:copyAuthFiles")}</span>
        <span className="text-xs text-muted-foreground">
          {filesAvailable
            ? filesLabel
            : t("executors:authFilesNotFoundOnThisMachine", { files: filesLabel })}
        </span>
      </div>
    </AuthOption>
  );
}

function EnvOption({
  method,
  isSelected,
  isDirty,
  secretId,
  baselineSecretId,
  showEnvVarInLabel,
  onSecretIdChange,
  secrets,
  onCheckedChange,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  isDirty: boolean;
  secretId: string | null;
  baselineSecretId: string | null;
  showEnvVarInLabel: boolean;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const label = showEnvVarInLabel
    ? t("executors:provideSecretForEnvVar", { envVar: method.env_var })
    : t("executors:provideSecret");
  return (
    <div>
      <AuthOption
        selected={isSelected}
        isDirty={isDirty}
        onCheckedChange={onCheckedChange}
        label={label}
      >
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium">{label}</span>
          <span className="text-xs text-muted-foreground">
            <Trans i18nKey="executors:setEnvVarViaStoredSecret" values={{ envVar: method.env_var }}>
              Set <code className="text-[11px] bg-muted px-1 rounded">{method.env_var}</code> via a
              stored secret
            </Trans>
          </span>
          {method.setup_hint && (
            <div className="markdown-body text-xs text-muted-foreground [&_p]:m-0">
              <ReactMarkdown>{method.setup_hint}</ReactMarkdown>
            </div>
          )}
        </div>
      </AuthOption>
      {isSelected && (
        <div className="pl-7 pt-2">
          <InlineSecretSelect
            secretId={secretId}
            onSecretIdChange={onSecretIdChange}
            secrets={secrets}
            placeholder={t("executors:selectOrCreateASecret")}
            isDirty={secretId !== baselineSecretId}
          />
        </div>
      )}
    </div>
  );
}

function AuthOption({
  selected,
  isDirty,
  onCheckedChange,
  label,
  children,
}: {
  selected: boolean;
  isDirty: boolean;
  onCheckedChange: (checked: boolean) => void;
  label: string;
  children: ReactNode;
}) {
  return (
    <label
      data-settings-dirty={isDirty}
      className={`${OPTION_LABEL_BASE} ${selected ? SELECTED_BORDER : DEFAULT_BORDER}`}
    >
      <Checkbox
        checked={selected}
        onCheckedChange={(checked) => onCheckedChange(checked === true)}
        aria-label={label}
        className="mt-0.5"
      />
      {children}
    </label>
  );
}

function setMethodSelected(selectedIds: Set<string>, methodId: string, selected: boolean) {
  if (selected) {
    selectedIds.add(methodId);
    return;
  }
  selectedIds.delete(methodId);
}

function AuthStatusBadge({
  fileSelected,
  envSelected,
  hasSecret,
}: {
  fileSelected: boolean;
  envSelected: boolean;
  hasSecret: boolean;
}) {
  const { t } = useTranslation();
  if (envSelected && hasSecret) {
    return (
      <Badge variant="default" className="bg-green-600 text-[10px] px-1.5 py-0">
        {t("executors:configured")}
      </Badge>
    );
  }
  if (fileSelected) {
    return (
      <Badge variant="default" className="bg-green-600 text-[10px] px-1.5 py-0">
        {t("executors:filesSelected")}
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
      {t("executors:notConfigured")}
    </Badge>
  );
}
