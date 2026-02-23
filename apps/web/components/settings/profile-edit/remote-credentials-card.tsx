"use client";

import { useEffect, useState } from "react";
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
  "flex items-start gap-3 rounded-md border p-3 cursor-pointer transition-colors";
const SELECTED_BORDER = "border-primary bg-primary/5";
const DEFAULT_BORDER = "border-border";
const RADIO_ITEM_CLASS =
  "mt-0.5 border border-muted-foreground/80 data-[state=checked]:border-primary";

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
  const handleToggle = (id: string, checked: boolean) => {
    onChange(checked ? [...selectedIds, id] : selectedIds.filter((v) => v !== id));
  };

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
              const envMethod = spec.methods.find((m) => m.type === "env");
              return (
                <AuthSection
                  key={spec.id}
                  spec={spec}
                  selectedIds={selectedSet}
                  onCredentialToggle={handleToggle}
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
  if (opts.envSecretId && opts.envMethod) return "env";
  return "none";
}

const AGENT_LOGO_IDS = new Set(["claude_code", "auggie", "codex", "gemini", "copilot", "amp"]);

function AuthSection({
  spec,
  selectedIds,
  onCredentialToggle,
  envSecretId,
  onMethodSecretChange,
  secrets,
}: {
  spec: RemoteAuthSpec;
  selectedIds: Set<string>;
  onCredentialToggle: (id: string, checked: boolean) => void;
  envSecretId: string | null;
  onMethodSecretChange: (methodId: string, secretId: string | null) => void;
  secrets: SecretListItem[];
}) {
  const envMethod = spec.methods.find((m) => m.type === "env");
  const fileMethod = spec.methods.find((m) => m.type === "files");
  const ghTokenMethod = spec.methods.find((m) => m.type === "gh_cli_token");
  const hasOnlyEnv = envMethod && !fileMethod && !ghTokenMethod;

  const [choice, setChoice] = useState<AuthChoice>(() =>
    initialChoice({ fileMethod, envMethod, ghTokenMethod, selectedIds, envSecretId }),
  );

  const handleChoice = (value: AuthChoice) => {
    setChoice(value);
    if (fileMethod) {
      onCredentialToggle(fileMethod.method_id, value === "files");
    }
    if (ghTokenMethod) {
      onCredentialToggle(ghTokenMethod.method_id, value === "gh_cli_token");
    }
    if (value !== "env" && envMethod) {
      onMethodSecretChange(envMethod.method_id, null);
    }
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
    <InlineSecretSelect
      secretId={secretId}
      onSecretIdChange={onSecretIdChange}
      secrets={secrets}
      label={envMethod.env_var}
      placeholder="Select or create a secret..."
    />
  );
}

function GhTokenOption({ method, isSelected }: { method: RemoteAuthMethod; isSelected: boolean }) {
  return (
    <label className={`${RADIO_LABEL_BASE} ${isSelected ? SELECTED_BORDER : DEFAULT_BORDER}`}>
      <RadioGroupItem value="gh_cli_token" className={RADIO_ITEM_CLASS} />
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{method.label ?? "Copy token from local CLI"}</span>
        {method.setup_hint && (
          <span className="text-xs text-muted-foreground">{method.setup_hint}</span>
        )}
      </div>
    </label>
  );
}

function FileOption({
  method,
  isSelected,
  filesAvailable,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  filesAvailable: boolean;
}) {
  const filesLabel = method.source_files?.join(", ") ?? "";
  return (
    <label
      className={`${RADIO_LABEL_BASE} ${
        isSelected ? SELECTED_BORDER : DEFAULT_BORDER
      } ${!filesAvailable ? "opacity-50 cursor-not-allowed" : ""}`}
    >
      <RadioGroupItem value="files" disabled={!filesAvailable} className={RADIO_ITEM_CLASS} />
      <div className="flex flex-col gap-0.5">
        <span className="text-sm font-medium">{method.label ?? "Copy auth files"}</span>
        <span className="text-xs text-muted-foreground">
          {filesLabel}
          {!filesAvailable && " — files not found on this machine"}
        </span>
      </div>
    </label>
  );
}

function EnvOption({
  method,
  isSelected,
  secretId,
  onSecretIdChange,
  secrets,
}: {
  method: RemoteAuthMethod;
  isSelected: boolean;
  secretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
}) {
  return (
    <div>
      <label className={`${RADIO_LABEL_BASE} ${isSelected ? SELECTED_BORDER : DEFAULT_BORDER}`}>
        <RadioGroupItem value="env" className={RADIO_ITEM_CLASS} />
        <div className="flex flex-col gap-0.5">
          <span className="text-sm font-medium">Provide secret</span>
          <span className="text-xs text-muted-foreground">
            Set <code className="text-[11px] bg-muted px-1 rounded">{method.env_var}</code> via a
            stored secret
          </span>
        </div>
      </label>
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
    <RadioGroup
      value={choice}
      onValueChange={(v) => onChoiceChange(v as AuthChoice)}
      className="gap-0"
    >
      {ghTokenMethod && (
        <GhTokenOption method={ghTokenMethod} isSelected={choice === "gh_cli_token"} />
      )}
      {fileMethod && (
        <FileOption
          method={fileMethod}
          isSelected={choice === "files"}
          filesAvailable={fileMethod.has_local_files ?? false}
        />
      )}
      {envMethod?.env_var && (
        <EnvOption
          method={envMethod}
          isSelected={choice === "env"}
          secretId={secretId}
          onSecretIdChange={onSecretIdChange}
          secrets={secrets}
        />
      )}
    </RadioGroup>
  );
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
