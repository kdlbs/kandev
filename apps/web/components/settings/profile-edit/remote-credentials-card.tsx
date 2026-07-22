"use client";

import { useEffect, useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import { IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { AgentLogo } from "@/components/agent-logo";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import { SettingsCard } from "@/components/settings/settings-card";
import {
  GitIdentityAccordionItem,
  type GitIdentityMode,
  type GitIdentityState,
} from "./git-identity-fields";
import {
  listRemoteCredentials,
  type RemoteAuthSpec,
  type RemoteAuthMethod,
} from "@/lib/api/domains/settings-api";
import type { SecretListItem } from "@/lib/types/http-secrets";

type AuthChoice = "files" | "env" | "none";
export type { GitIdentityMode, GitIdentityState } from "./git-identity-fields";

const RADIO_LABEL_BASE =
  "flex w-full items-start gap-3 rounded-md border p-3 text-left cursor-pointer transition-colors";
const SELECTED_BORDER = "border-primary bg-primary/5";
const DEFAULT_BORDER = "border-border";
const OPTION_DOT_BASE =
  "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border";

type RemoteCredentialsCardProps = {
  isRemote: boolean;
  selectedIds: string[];
  baselineSelectedIds?: string[];
  onChange: (ids: string[]) => void;
  agentEnvVars: Record<string, string | null>;
  baselineAgentEnvVars?: Record<string, string | null>;
  onAgentEnvVarChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
  gitIdentityMode: GitIdentityMode;
  baselineGitIdentityMode?: GitIdentityMode;
  onGitIdentityModeChange: (mode: GitIdentityMode) => void;
  gitUserName: string;
  gitUserEmail: string;
  baselineGitUserName?: string;
  baselineGitUserEmail?: string;
  onGitUserNameChange: (value: string) => void;
  onGitUserEmailChange: (value: string) => void;
  localGitIdentity: GitIdentityState;
};

export function RemoteCredentialsCard({
  isRemote,
  selectedIds,
  baselineSelectedIds = [],
  onChange,
  agentEnvVars,
  baselineAgentEnvVars = {},
  onAgentEnvVarChange,
  secrets,
  gitIdentityMode,
  baselineGitIdentityMode = "override",
  onGitIdentityModeChange,
  gitUserName,
  gitUserEmail,
  baselineGitUserName = "",
  baselineGitUserEmail = "",
  onGitUserNameChange,
  onGitUserEmailChange,
  localGitIdentity,
}: RemoteCredentialsCardProps) {
  const [authSpecs, setAuthSpecs] = useState<RemoteAuthSpec[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    listRemoteCredentials()
      .then((res) => setAuthSpecs(res.auth_specs ?? []))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const selectedSet = new Set(selectedIds);
  const baselineSelectedSet = new Set(baselineSelectedIds);
  const credentialsDirty = !sameStringSet(selectedSet, baselineSelectedSet);
  const agentEnvVarsDirty = JSON.stringify(agentEnvVars) !== JSON.stringify(baselineAgentEnvVars);
  const gitIdentityDirty =
    gitIdentityMode !== baselineGitIdentityMode ||
    (gitIdentityMode === "override" &&
      (gitUserName !== baselineGitUserName || gitUserEmail !== baselineGitUserEmail));
  const isDirty = credentialsDirty || agentEnvVarsDirty || gitIdentityDirty;

  if (loading) {
    return <RemoteCredentialsLoading isDirty={isDirty} />;
  }

  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle>Remote Credentials</CardTitle>
        <CardDescription>
          Configure authentication for tools and agents in the remote environment.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {authSpecs.length > 0 || isRemote ? (
          <Accordion type="multiple">
            {isRemote && (
              <GitIdentityAccordionItem
                mode={gitIdentityMode}
                baselineMode={baselineGitIdentityMode}
                onModeChange={onGitIdentityModeChange}
                gitUserName={gitUserName}
                gitUserEmail={gitUserEmail}
                baselineGitUserName={baselineGitUserName}
                baselineGitUserEmail={baselineGitUserEmail}
                onGitUserNameChange={onGitUserNameChange}
                onGitUserEmailChange={onGitUserEmailChange}
                localGitIdentity={localGitIdentity}
              />
            )}
            {authSpecs.map((spec) => {
              const methods = getSpecMethods(spec);
              const envMethod = methods.find((m) => m.type === "env");
              return (
                <AuthSection
                  key={spec.id}
                  spec={spec}
                  selectedIds={selectedSet}
                  baselineSelectedIds={baselineSelectedSet}
                  onCredentialsChange={onChange}
                  envSecretId={envMethod ? (agentEnvVars[envMethod.method_id] ?? null) : null}
                  baselineEnvSecretId={
                    envMethod ? (baselineAgentEnvVars[envMethod.method_id] ?? null) : null
                  }
                  onMethodSecretChange={onAgentEnvVarChange}
                  secrets={secrets}
                />
              );
            })}
          </Accordion>
        ) : (
          <p className="text-sm text-muted-foreground">No transferable credentials found.</p>
        )}
      </CardContent>
    </SettingsCard>
  );
}

function RemoteCredentialsLoading({ isDirty }: { isDirty: boolean }) {
  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle>Remote Credentials</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <IconLoader2 className="h-4 w-4 animate-spin" />
          Loading...
        </div>
      </CardContent>
    </SettingsCard>
  );
}

function sameStringSet(left: Set<string>, right: Set<string>): boolean {
  return left.size === right.size && [...left].every((value) => right.has(value));
}

function getSpecMethods(spec: RemoteAuthSpec): RemoteAuthMethod[] {
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
  // Treat env as selected either when the user just clicked the radio (its
  // method id is in selectedIds, no secret picked yet) or when a secret is
  // already persisted. Without the selectedIds branch, first-time env setup
  // for an agent that exposes both `files` and `env` methods is broken: the
  // radio click never updates state, `choice` re-derives to "none", the env
  // option deselects, and the secret dropdown disappears before the user can
  // pick anything.
  if (opts.envMethod && (opts.selectedIds.has(opts.envMethod.method_id) || opts.envSecretId))
    return "env";
  return "none";
}

const AGENT_LOGO_IDS = new Set(["claude_code", "auggie", "codex", "gemini", "copilot", "amp"]);

function AuthSection({
  spec,
  selectedIds,
  baselineSelectedIds,
  onCredentialsChange,
  envSecretId,
  baselineEnvSecretId,
  onMethodSecretChange,
  secrets,
}: {
  spec: RemoteAuthSpec;
  selectedIds: Set<string>;
  baselineSelectedIds: Set<string>;
  onCredentialsChange: (ids: string[]) => void;
  envSecretId: string | null;
  baselineEnvSecretId: string | null;
  onMethodSecretChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
}) {
  const methods = getSpecMethods(spec);
  const envMethod = methods.find((m) => m.type === "env");
  const fileMethod = methods.find((m) => m.type === "files");
  const hasOnlyEnv = envMethod && !fileMethod;

  // `choice` is derived from props so the configured-status badge updates live
  // when the user picks a secret in the dropdown (which only flows back through
  // `envSecretId`). Holding it in useState would freeze the badge to its initial
  // value until a full page reload.
  const choice: AuthChoice = initialChoice({
    fileMethod,
    envMethod,
    selectedIds,
    envSecretId,
  });
  const baselineChoice = initialChoice({
    fileMethod,
    envMethod,
    selectedIds: baselineSelectedIds,
    envSecretId: baselineEnvSecretId,
  });
  const isDirty = choice !== baselineChoice || envSecretId !== baselineEnvSecretId;

  const handleChoice = (value: AuthChoice) => {
    const nextSelectedIds = new Set(selectedIds);
    if (fileMethod) {
      setMethodSelected(nextSelectedIds, fileMethod.method_id, value === "files");
    }
    if (envMethod) {
      // Track env in selectedIds so `initialChoice` stays "env" while the
      // user is still picking a secret. Switching away clears the secret too.
      setMethodSelected(nextSelectedIds, envMethod.method_id, value === "env");
      if (value !== "env") {
        onMethodSecretChange(envMethod.method_id, null);
      }
    }
    onCredentialsChange([...nextSelectedIds]);
  };

  const showLogo = AGENT_LOGO_IDS.has(spec.id);

  return (
    <AccordionItem value={spec.id} data-settings-dirty={isDirty}>
      <AccordionTrigger>
        <div className="flex items-center gap-2 flex-1">
          {showLogo && <AgentLogo agentName={spec.id} size={18} />}
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
        placeholder="Select or create a secret..."
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
  const filesLabel = method.source_files?.join(", ") ?? "";
  return (
    <AuthOptionButton
      selected={isSelected}
      isDirty={isDirty}
      onSelect={onSelect}
      label={method.label ?? "Copy auth files"}
    >
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{method.label ?? "Copy auth files"}</span>
        <span className="text-xs text-muted-foreground">
          {filesLabel}
          {!filesAvailable && " — files not found on this machine"}
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
  return (
    <div>
      <AuthOptionButton
        selected={isSelected}
        isDirty={isDirty}
        onSelect={onSelect}
        label="Provide secret"
      >
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium">Provide secret</span>
          <span className="text-xs text-muted-foreground">
            Set <code className="text-[11px] bg-muted px-1 rounded">{method.env_var}</code> via a
            stored secret
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
            placeholder="Select or create a secret..."
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
  return (
    <div role="radiogroup" aria-label="Remote auth method" className="grid gap-0">
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
  if (choice === "env" && hasSecret) {
    return (
      <Badge variant="default" className="bg-green-600 text-[10px] px-1.5 py-0">
        Configured
      </Badge>
    );
  }
  if (choice === "files") {
    return (
      <Badge variant="default" className="bg-green-600 text-[10px] px-1.5 py-0">
        Files Selected
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
      Not Configured
    </Badge>
  );
}
