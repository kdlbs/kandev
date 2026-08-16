"use client";

import { Trans, useTranslation } from "react-i18next";
import ReactMarkdown from "react-markdown";
import type { ReactNode } from "react";
import { Badge } from "@kandev/ui/badge";
import { AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { AgentLogo } from "@/components/agent-logo";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import type { RemoteAuthMethod, RemoteAuthSpec } from "@/lib/api/domains/settings-api";
import type { AgentConfigBundle } from "@/lib/api/domains/agent-config-api";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { AgentConfigOptions } from "./portable-config-bundles";

type AuthChoice = "files" | "env" | "none";

const RADIO_LABEL_BASE =
  "flex w-full items-start gap-3 rounded-md border p-3 text-left cursor-pointer transition-colors";
const SELECTED_BORDER = "border-primary bg-primary/5";
const DEFAULT_BORDER = "border-border";
const OPTION_DOT_BASE =
  "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border";
const AGENT_LOGO_IDS = new Set(["claude_code", "auggie", "codex", "gemini", "copilot", "amp"]);

export function getSpecMethods(spec: RemoteAuthSpec): RemoteAuthMethod[] {
  return Array.isArray(spec.methods) ? spec.methods : [];
}

type InitialChoiceOpts = {
  fileMethod: RemoteAuthMethod | undefined;
  envMethod: RemoteAuthMethod | undefined;
  selectedIds: Set<string>;
  envSecretId: string | null;
};

function initialChoice(opts: InitialChoiceOpts): AuthChoice {
  if (opts.fileMethod && opts.selectedIds.has(opts.fileMethod.method_id)) return "files";
  // Keep env selected while the user is choosing a secret. Otherwise the
  // secret dropdown disappears before the first secret can be selected.
  if (opts.envMethod && (opts.selectedIds.has(opts.envMethod.method_id) || opts.envSecretId)) {
    return "env";
  }
  return "none";
}

type AuthSectionProps = {
  spec: RemoteAuthSpec;
  selectedIds: Set<string>;
  baselineSelectedIds: Set<string>;
  onCredentialsChange: (ids: string[]) => void;
  envSecretId: string | null;
  baselineEnvSecretId: string | null;
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
  envSecretId,
  baselineEnvSecretId,
  onMethodSecretChange,
  secrets,
  configBundles,
  configBundleIds,
  baselineConfigBundleIds,
  onConfigBundleChange,
  isSSH,
}: AuthSectionProps) {
  const methods = getSpecMethods(spec);
  const envMethod = methods.find((m) => m.type === "env");
  const fileMethod = methods.find((m) => m.type === "files");
  const hasOnlyEnv = envMethod && !fileMethod;
  const choice = initialChoice({ fileMethod, envMethod, selectedIds, envSecretId });
  const baselineChoice = initialChoice({
    fileMethod,
    envMethod,
    selectedIds: baselineSelectedIds,
    envSecretId: baselineEnvSecretId,
  });
  const isDirty = choice !== baselineChoice || envSecretId !== baselineEnvSecretId;
  const configIsDirty = configBundles.some(
    (bundle) => configBundleIds.includes(bundle.id) !== baselineConfigBundleIds.includes(bundle.id),
  );

  const handleChoice = (value: AuthChoice) => {
    const nextSelectedIds = new Set(selectedIds);
    if (fileMethod) setMethodSelected(nextSelectedIds, fileMethod.method_id, value === "files");
    if (envMethod) {
      setMethodSelected(nextSelectedIds, envMethod.method_id, value === "env");
      if (value !== "env") onMethodSecretChange(envMethod.method_id, null);
    }
    onCredentialsChange([...nextSelectedIds]);
  };

  return (
    <AccordionItem value={spec.id} data-settings-dirty={isDirty || configIsDirty}>
      <AccordionTrigger>
        <div className="flex items-center gap-2 flex-1">
          {AGENT_LOGO_IDS.has(spec.id) && <AgentLogo agentName={spec.id} size={18} />}
          <span className="font-medium text-sm">{spec.display_name}</span>
          <AuthStatusBadge choice={choice} hasSecret={!!envSecretId} />
        </div>
      </AccordionTrigger>
      <AccordionContent className="h-auto">
        <div className="space-y-3 text-sm">
          {hasOnlyEnv && envMethod ? (
            <EnvOnlySection
              envMethod={envMethod}
              secretId={envSecretId}
              baselineSecretId={baselineEnvSecretId}
              onSecretIdChange={(sid) => onMethodSecretChange(envMethod.method_id, sid)}
              secrets={secrets}
            />
          ) : (
            <AuthChoiceRadio
              choice={choice}
              baselineChoice={baselineChoice}
              onChoiceChange={handleChoice}
              fileMethod={fileMethod}
              envMethod={envMethod}
              secretId={envSecretId}
              baselineSecretId={baselineEnvSecretId}
              onSecretIdChange={(sid) => {
                if (envMethod) onMethodSecretChange(envMethod.method_id, sid);
              }}
              secrets={secrets}
            />
          )}
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

function EnvOnlySection({
  envMethod,
  secretId,
  baselineSecretId,
  onSecretIdChange,
  secrets,
}: {
  envMethod: RemoteAuthMethod;
  secretId: string | null;
  baselineSecretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
}) {
  const { t } = useTranslation();
  return (
    <>
      {envMethod.setup_hint && (
        <div className="markdown-body text-xs text-muted-foreground [&_p]:m-0">
          <ReactMarkdown>{envMethod.setup_hint}</ReactMarkdown>
        </div>
      )}
      <InlineSecretSelect
        secretId={secretId}
        onSecretIdChange={onSecretIdChange}
        secrets={secrets}
        label={envMethod.env_var}
        placeholder={t("executors:selectOrCreateASecret")}
        isDirty={secretId !== baselineSecretId}
      />
    </>
  );
}

function FileOption({
  method,
  isSelected,
  isDirty,
  filesAvailable,
  onSelect,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  isDirty: boolean;
  filesAvailable: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const filesLabel = method.source_files?.join(", ") ?? "";
  return (
    <AuthOptionButton
      selected={isSelected}
      isDirty={isDirty}
      onSelect={onSelect}
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
    </AuthOptionButton>
  );
}

function EnvOption({
  method,
  isSelected,
  isDirty,
  secretId,
  baselineSecretId,
  onSecretIdChange,
  secrets,
  onSelect,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  isDirty: boolean;
  secretId: string | null;
  baselineSecretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div>
      <AuthOptionButton
        selected={isSelected}
        isDirty={isDirty}
        onSelect={onSelect}
        label={t("executors:provideSecret")}
      >
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium">{t("executors:provideSecret")}</span>
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
      </AuthOptionButton>
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

function AuthChoiceRadio({
  choice,
  baselineChoice,
  onChoiceChange,
  fileMethod,
  envMethod,
  secretId,
  baselineSecretId,
  onSecretIdChange,
  secrets,
}: {
  choice: AuthChoice;
  baselineChoice: AuthChoice;
  onChoiceChange: (v: AuthChoice) => void;
  fileMethod?: RemoteAuthMethod;
  envMethod?: RemoteAuthMethod;
  secretId: string | null;
  baselineSecretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
}) {
  const { t } = useTranslation();
  return (
    <div role="radiogroup" aria-label={t("executors:remoteAuthMethod")} className="grid gap-0">
      {fileMethod && (
        <FileOption
          method={fileMethod}
          isSelected={choice === "files"}
          isDirty={(choice === "files") !== (baselineChoice === "files")}
          filesAvailable={fileMethod.has_local_files ?? false}
          onSelect={() => onChoiceChange("files")}
        />
      )}
      {envMethod?.env_var && (
        <EnvOption
          method={envMethod}
          isSelected={choice === "env"}
          isDirty={(choice === "env") !== (baselineChoice === "env")}
          secretId={secretId}
          baselineSecretId={baselineSecretId}
          onSecretIdChange={onSecretIdChange}
          secrets={secrets}
          onSelect={() => onChoiceChange("env")}
        />
      )}
    </div>
  );
}

function AuthOptionButton({
  selected,
  isDirty,
  onSelect,
  label,
  children,
}: {
  selected: boolean;
  isDirty: boolean;
  onSelect: () => void;
  label: string;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      aria-label={label}
      onClick={onSelect}
      data-settings-dirty={isDirty}
      className={`${RADIO_LABEL_BASE} ${selected ? SELECTED_BORDER : DEFAULT_BORDER}`}
    >
      <span
        aria-hidden="true"
        className={`${OPTION_DOT_BASE} ${selected ? "border-primary bg-primary text-primary-foreground" : "border-muted-foreground/80"}`}
      >
        {selected && <span className="size-2 rounded-full bg-current" />}
      </span>
      {children}
    </button>
  );
}

function setMethodSelected(selectedIds: Set<string>, methodId: string, selected: boolean) {
  if (selected) {
    selectedIds.add(methodId);
    return;
  }
  selectedIds.delete(methodId);
}

function AuthStatusBadge({ choice, hasSecret }: { choice: AuthChoice; hasSecret: boolean }) {
  const { t } = useTranslation();
  if (choice === "env" && hasSecret) {
    return (
      <Badge variant="default" className="bg-green-600 text-[10px] px-1.5 py-0">
        {t("executors:configured")}
      </Badge>
    );
  }
  if (choice === "files") {
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
