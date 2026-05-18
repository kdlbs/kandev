"use client";

import { useEffect, useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import { IconLoader2 } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@kandev/ui/accordion";
import { AgentLogo } from "@/components/agent-logo";
import { InlineSecretSelect } from "@/components/settings/profile-edit/inline-secret-select";
import {
  listRemoteCredentials,
  type RemoteAuthSpec,
  type RemoteAuthMethod,
} from "@/lib/api/domains/settings-api";
import type { SecretListItem } from "@/lib/types/http-secrets";

type AuthChoice = "files" | "env" | "gh_cli_token" | "none";
export type GitIdentityMode = "local" | "override";
export type GitIdentityState = {
  userName: string;
  userEmail: string;
  detected: boolean;
};

const RADIO_LABEL_BASE =
  "flex w-full items-start gap-3 rounded-md border p-3 text-left cursor-pointer transition-colors";
const SELECTED_BORDER = "border-primary bg-primary/5";
const DEFAULT_BORDER = "border-border";
const RADIO_ITEM_CLASS =
  "mt-0.5 border border-muted-foreground/80 data-[state=checked]:border-primary";
const OPTION_DOT_BASE =
  "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full border";

type RemoteCredentialsCardProps = {
  isRemote: boolean;
  selectedIds: string[];
  onChange: (ids: string[]) => void;
  agentEnvVars: Record<string, string | null>;
  onAgentEnvVarChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
  gitIdentityMode: GitIdentityMode;
  onGitIdentityModeChange: (mode: GitIdentityMode) => void;
  gitUserName: string;
  gitUserEmail: string;
  onGitUserNameChange: (value: string) => void;
  onGitUserEmailChange: (value: string) => void;
  localGitIdentity: GitIdentityState;
};

export function RemoteCredentialsCard({
  isRemote,
  selectedIds,
  onChange,
  agentEnvVars,
  onAgentEnvVarChange,
  secrets,
  gitIdentityMode,
  onGitIdentityModeChange,
  gitUserName,
  gitUserEmail,
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

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Remote Credentials</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <IconLoader2 className="h-4 w-4 animate-spin" />
            Loading...
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
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
                onModeChange={onGitIdentityModeChange}
                gitUserName={gitUserName}
                gitUserEmail={gitUserEmail}
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
                  onCredentialsChange={onChange}
                  envSecretId={envMethod ? (agentEnvVars[envMethod.method_id] ?? null) : null}
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
    </Card>
  );
}

function getSpecMethods(spec: RemoteAuthSpec): RemoteAuthMethod[] {
  return Array.isArray(spec.methods) ? spec.methods : [];
}

function GitIdentityAccordionItem({
  mode,
  onModeChange,
  gitUserName,
  gitUserEmail,
  onGitUserNameChange,
  onGitUserEmailChange,
  localGitIdentity,
}: {
  mode: GitIdentityMode;
  onModeChange: (mode: GitIdentityMode) => void;
  gitUserName: string;
  gitUserEmail: string;
  onGitUserNameChange: (value: string) => void;
  onGitUserEmailChange: (value: string) => void;
  localGitIdentity: GitIdentityState;
}) {
  const isLocalAutoDetected = mode === "local" && localGitIdentity.detected;
  let badgeLabel = "Custom";
  if (isLocalAutoDetected) {
    badgeLabel = "Auto-detect";
  } else if (mode === "local") {
    badgeLabel = "Not Configured";
  }
  const localIdentityDescription = localGitIdentity.detected
    ? `${localGitIdentity.userName} <${localGitIdentity.userEmail}>`
    : "Local git user.name/user.email not detected on this machine";
  const badgeClassName = isLocalAutoDetected
    ? "bg-green-600 text-[10px] px-1.5 py-0"
    : "text-[10px] px-1.5 py-0";
  const badgeVariant = isLocalAutoDetected ? "default" : "secondary";

  return (
    <AccordionItem value="git_identity">
      <AccordionTrigger>
        <div className="flex items-center gap-2 flex-1">
          <span className="font-medium text-sm">Git Identity</span>
          <Badge variant={badgeVariant} className={badgeClassName}>
            {badgeLabel}
          </Badge>
        </div>
      </AccordionTrigger>
      <AccordionContent className="h-auto">
        <div className="space-y-3 text-sm">
          <p className="text-xs text-muted-foreground">
            Used by remote executors for commit author configuration.
          </p>
          <RadioGroup
            value={mode}
            onValueChange={(value) => onModeChange(value as GitIdentityMode)}
            className="gap-2"
          >
            <label
              className={`${RADIO_LABEL_BASE} ${mode === "local" ? SELECTED_BORDER : DEFAULT_BORDER}`}
            >
              <RadioGroupItem
                value="local"
                disabled={!localGitIdentity.detected}
                className={RADIO_ITEM_CLASS}
              />
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium">Use local git config</span>
                <span className="text-xs text-muted-foreground">{localIdentityDescription}</span>
              </div>
            </label>
            <label
              className={`${RADIO_LABEL_BASE} ${mode === "override" ? SELECTED_BORDER : DEFAULT_BORDER}`}
            >
              <RadioGroupItem value="override" className={RADIO_ITEM_CLASS} />
              <div className="flex flex-col gap-0.5">
                <span className="text-sm font-medium">Override identity</span>
                <span className="text-xs text-muted-foreground">
                  Set a custom name and email for remote git commits.
                </span>
              </div>
            </label>
          </RadioGroup>
          {mode === "override" && (
            <OverrideIdentityFields
              gitUserName={gitUserName}
              gitUserEmail={gitUserEmail}
              onGitUserNameChange={onGitUserNameChange}
              onGitUserEmailChange={onGitUserEmailChange}
            />
          )}
        </div>
      </AccordionContent>
    </AccordionItem>
  );
}

function OverrideIdentityFields({
  gitUserName,
  gitUserEmail,
  onGitUserNameChange,
  onGitUserEmailChange,
}: {
  gitUserName: string;
  gitUserEmail: string;
  onGitUserNameChange: (value: string) => void;
  onGitUserEmailChange: (value: string) => void;
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="space-y-1.5">
        <Label htmlFor="remote-git-user-name">Git User Name</Label>
        <Input
          id="remote-git-user-name"
          value={gitUserName}
          onChange={(e) => onGitUserNameChange(e.target.value)}
          placeholder="Jane Developer"
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="remote-git-user-email">Git User Email</Label>
        <Input
          id="remote-git-user-email"
          value={gitUserEmail}
          onChange={(e) => onGitUserEmailChange(e.target.value)}
          placeholder="jane@example.com"
        />
      </div>
    </div>
  );
}

type InitialChoiceOpts = {
  fileMethod: RemoteAuthMethod | undefined;
  envMethod: RemoteAuthMethod | undefined;
  ghTokenMethod: RemoteAuthMethod | undefined;
  selectedIds: Set<string>;
  envSecretId: string | null;
};

function initialChoice(opts: InitialChoiceOpts): AuthChoice {
  if (opts.ghTokenMethod && opts.selectedIds.has(opts.ghTokenMethod.method_id))
    return "gh_cli_token";
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
  onCredentialsChange,
  envSecretId,
  onMethodSecretChange,
  secrets,
}: {
  spec: RemoteAuthSpec;
  selectedIds: Set<string>;
  onCredentialsChange: (ids: string[]) => void;
  envSecretId: string | null;
  onMethodSecretChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
}) {
  const methods = getSpecMethods(spec);
  const envMethod = methods.find((m) => m.type === "env");
  const fileMethod = methods.find((m) => m.type === "files");
  const ghTokenMethod = methods.find((m) => m.type === "gh_cli_token");
  const hasOnlyEnv = envMethod && !fileMethod && !ghTokenMethod;

  // `choice` is derived from props so the configured-status badge updates live
  // when the user picks a secret in the dropdown (which only flows back through
  // `envSecretId`). Holding it in useState would freeze the badge to its initial
  // value until a full page reload.
  const choice: AuthChoice = initialChoice({
    fileMethod,
    envMethod,
    ghTokenMethod,
    selectedIds,
    envSecretId,
  });

  const handleChoice = (value: AuthChoice) => {
    const nextSelectedIds = new Set(selectedIds);
    if (fileMethod) {
      setMethodSelected(nextSelectedIds, fileMethod.method_id, value === "files");
    }
    if (ghTokenMethod) {
      setMethodSelected(nextSelectedIds, ghTokenMethod.method_id, value === "gh_cli_token");
    }
    if (envMethod) {
      // Track env in selectedIds the same way `files`/`gh_cli_token` are
      // tracked, so `initialChoice` stays "env" while the user is still
      // picking a secret. Switching away clears the secret too.
      setMethodSelected(nextSelectedIds, envMethod.method_id, value === "env");
      if (value !== "env") {
        onMethodSecretChange(envMethod.method_id, null);
      }
    }
    onCredentialsChange([...nextSelectedIds]);
  };

  const showLogo = AGENT_LOGO_IDS.has(spec.id);

  return (
    <AccordionItem value={spec.id}>
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
              onSecretIdChange={(sid) => onMethodSecretChange(envMethod.method_id, sid)}
              secrets={secrets}
            />
          ) : (
            <AuthChoiceRadio
              choice={choice}
              onChoiceChange={handleChoice}
              fileMethod={fileMethod}
              envMethod={envMethod}
              ghTokenMethod={ghTokenMethod}
              secretId={envSecretId}
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
  onSecretIdChange,
  secrets,
}: {
  envMethod: RemoteAuthMethod;
  secretId: string | null;
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
      />
    </>
  );
}

function GhTokenOption({
  method,
  isSelected,
  onSelect,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  onSelect: () => void;
}) {
  return (
    <AuthOptionButton
      selected={isSelected}
      onSelect={onSelect}
      label={method.label ?? "Copy token from local CLI"}
    >
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{method.label ?? "Copy token from local CLI"}</span>
        {method.setup_hint && (
          <div className="markdown-body text-xs text-muted-foreground [&_p]:m-0">
            <ReactMarkdown>{method.setup_hint}</ReactMarkdown>
          </div>
        )}
      </div>
    </AuthOptionButton>
  );
}

function FileOption({
  method,
  isSelected,
  filesAvailable,
  onSelect,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  filesAvailable: boolean;
  onSelect: () => void;
}) {
  const filesLabel = method.source_files?.join(", ") ?? "";
  return (
    <AuthOptionButton
      selected={isSelected}
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
  secretId,
  onSecretIdChange,
  secrets,
  onSelect,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  secretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
  onSelect: () => void;
}) {
  return (
    <div>
      <AuthOptionButton selected={isSelected} onSelect={onSelect} label="Provide secret">
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
          />
        </div>
      )}
    </div>
  );
}

function AuthChoiceRadio({
  choice,
  onChoiceChange,
  fileMethod,
  envMethod,
  ghTokenMethod,
  secretId,
  onSecretIdChange,
  secrets,
}: {
  choice: AuthChoice;
  onChoiceChange: (v: AuthChoice) => void;
  fileMethod?: RemoteAuthMethod;
  envMethod?: RemoteAuthMethod;
  ghTokenMethod?: RemoteAuthMethod;
  secretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
}) {
  return (
    <div role="radiogroup" aria-label="Remote auth method" className="grid gap-0">
      {ghTokenMethod && (
        <GhTokenOption
          method={ghTokenMethod}
          isSelected={choice === "gh_cli_token"}
          onSelect={() => onChoiceChange("gh_cli_token")}
        />
      )}
      {fileMethod && (
        <FileOption
          method={fileMethod}
          isSelected={choice === "files"}
          filesAvailable={fileMethod.has_local_files ?? false}
          onSelect={() => onChoiceChange("files")}
        />
      )}
      {envMethod?.env_var && (
        <EnvOption
          method={envMethod}
          isSelected={choice === "env"}
          secretId={secretId}
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
  onSelect,
  label,
  children,
}: {
  selected: boolean;
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
  if (choice === "gh_cli_token") {
    return (
      <Badge variant="default" className="bg-green-600 text-[10px] px-1.5 py-0">
        Auto-detect
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
      Not Configured
    </Badge>
  );
}
