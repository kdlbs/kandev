"use client";

import { use, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { useAppStore } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import {
  createExecutorProfile,
  fetchLocalGitIdentity,
  fetchDefaultScripts,
  listScriptPlaceholders,
} from "@/lib/api/domains/settings-api";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
import { EXECUTOR_ICON_MAP, getExecutorLabel } from "@/lib/executor-icons";
import { ProfileDetailsCard } from "@/components/settings/profile-edit/profile-details-card";
import {
  McpPolicyCard,
  validateMcpPolicy,
} from "@/components/settings/profile-edit/mcp-policy-card";
import {
  EnvVarsCard,
  useEnvVarRows,
  rowsToEnvVars,
} from "@/components/settings/profile-edit/env-vars-card";
import { ScriptCard } from "@/components/settings/profile-edit/script-card";
import { DockerfileBuildCard } from "@/components/settings/profile-edit/docker-sections";
import { SpritesApiKeyCard } from "@/components/settings/profile-edit/sprites-api-key-card";
import { NetworkPoliciesCard } from "@/components/settings/profile-edit/sprites-sections";
import {
  RemoteCredentialsCard,
  type GitIdentityMode,
  type GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import type { Executor, ProfileEnvVar } from "@/lib/types/http";

const EXECUTORS_ROUTE = "/settings/executors";
const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";

const EXECUTOR_TYPE_MAP: Record<
  string,
  { executorId: string; label: string; description: string }
> = {
  local: {
    executorId: "exec-local",
    label: "Local",
    description: "Runs agents directly in the repository folder.",
  },
  worktree: {
    executorId: "exec-worktree",
    label: "Worktree",
    description: "Creates git worktrees for isolated agent sessions.",
  },
  local_docker: {
    executorId: "exec-local-docker",
    label: "Docker",
    description: "Runs Docker containers on this machine.",
  },
  remote_docker: {
    executorId: "exec-remote-docker",
    label: "Remote Docker",
    description: "Connects to a remote Docker host.",
  },
  sprites: {
    executorId: "exec-sprites",
    label: "Sprites.dev",
    description: "Runs agents in Sprites.dev cloud sandboxes.",
  },
};

const DefaultIcon = EXECUTOR_ICON_MAP.local;

function ExecutorTypeIcon({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return <Icon className="h-5 w-5 text-muted-foreground" />;
}

export default function CreateProfilePage({ params }: { params: Promise<{ type: string }> }) {
  const { type } = use(params);
  const typeInfo = EXECUTOR_TYPE_MAP[type];

  if (!typeInfo) {
    return <InvalidTypeFallback />;
  }

  return <CreateProfileForm executorType={type} typeInfo={typeInfo} />;
}

function InvalidTypeFallback() {
  const router = useRouter();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-muted-foreground">Unknown executor type</p>
        <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
          Back to Executors
        </Button>
      </CardContent>
    </Card>
  );
}

function CreateProfileHeader({
  type,
  label,
  description,
}: {
  type: string;
  label: string;
  description: string;
}) {
  const router = useRouter();
  return (
    <>
      <div className="flex items-start justify-between flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <ExecutorTypeIcon type={type} />
            <h2 className="text-2xl font-bold">New {label} Profile</h2>
            <Badge variant="outline" className="text-xs">
              {getExecutorLabel(type)}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="cursor-pointer"
        >
          Back to Executors
        </Button>
      </div>
      <Separator />
    </>
  );
}

function CreateFormActions({
  saving,
  saveDisabled,
  onSave,
}: {
  saving: boolean;
  saveDisabled: boolean;
  onSave: () => void;
}) {
  const router = useRouter();
  return (
    <div className="flex items-center justify-end gap-2">
      <Button
        variant="outline"
        onClick={() => router.push(EXECUTORS_ROUTE)}
        className="cursor-pointer"
      >
        Cancel
      </Button>
      <Button onClick={onSave} disabled={saveDisabled} className="cursor-pointer">
        {saving ? "Creating..." : "Create Profile"}
      </Button>
    </div>
  );
}

type BuildProfileConfigInput = {
  isRemote: boolean;
  isSprites: boolean;
  networkPolicyRules: NetworkPolicyRule[];
  remoteCredentials: string[];
  agentEnvVars: Record<string, string | null>;
  gitIdentityMode: GitIdentityMode;
  localGitIdentity: GitIdentityState;
  gitUserName: string;
  gitUserEmail: string;
};

function buildProfileConfig(input: BuildProfileConfigInput): Record<string, string> | undefined {
  const {
    isRemote,
    isSprites,
    networkPolicyRules,
    remoteCredentials,
    agentEnvVars,
    gitIdentityMode,
    localGitIdentity,
    gitUserName,
    gitUserEmail,
  } = input;
  const config: Record<string, string> = {};
  if (isSprites && networkPolicyRules.length > 0) {
    config.sprites_network_policy_rules = JSON.stringify(networkPolicyRules);
  }
  if (isSprites && remoteCredentials.length > 0) {
    config.remote_credentials = JSON.stringify(remoteCredentials);
  }
  const nonNullEnvVars = Object.fromEntries(
    Object.entries(agentEnvVars).filter(([, v]) => v != null),
  );
  if (isSprites && Object.keys(nonNullEnvVars).length > 0) {
    config.remote_auth_secrets = JSON.stringify(nonNullEnvVars);
  }
  if (isRemote) {
    const effectiveName =
      gitIdentityMode === "local" ? localGitIdentity.userName.trim() : gitUserName.trim();
    const effectiveEmail =
      gitIdentityMode === "local" ? localGitIdentity.userEmail.trim() : gitUserEmail.trim();
    if (effectiveName) {
      config.git_user_name = effectiveName;
    }
    if (effectiveEmail) {
      config.git_user_email = effectiveEmail;
    }
  }
  return Object.keys(config).length > 0 ? config : undefined;
}

function useDefaultScripts(executorType: string, setPrepareScript: (v: string) => void) {
  useEffect(() => {
    fetchDefaultScripts(executorType)
      .then((res) => {
        if (res.prepare_script) setPrepareScript(res.prepare_script);
      })
      .catch(() => {});
  }, [executorType, setPrepareScript]);
}

function useCreateRemoteFlags(executorType: string) {
  const isRemote =
    executorType === "local_docker" ||
    executorType === "remote_docker" ||
    executorType === "sprites";
  return {
    isRemote,
    isDocker: executorType === "local_docker" || executorType === "remote_docker",
    isSprites: executorType === "sprites",
  };
}

function useCreateRemoteAuthState(executorType: string) {
  const [remoteCredentials, setRemoteCredentials] = useState<string[]>(() =>
    executorType === "sprites" ? ["gh_cli_token"] : [],
  );
  const [agentEnvVars, setAgentEnvVars] = useState<Record<string, string | null>>({});
  const [networkPolicyRules, setNetworkPolicyRules] = useState<NetworkPolicyRule[]>([]);

  const handleAgentEnvVarChange = useCallback((agentId: string, secretId: string | null) => {
    setAgentEnvVars((prev) => ({ ...prev, [agentId]: secretId }));
  }, []);

  return {
    remoteCredentials,
    setRemoteCredentials,
    agentEnvVars,
    handleAgentEnvVarChange,
    networkPolicyRules,
    setNetworkPolicyRules,
  };
}

function useCreateGitIdentityState(isRemote: boolean) {
  const [localGitIdentity, setLocalGitIdentity] = useState<GitIdentityState>({
    userName: "",
    userEmail: "",
    detected: false,
  });
  const [gitIdentityMode, setGitIdentityMode] = useState<GitIdentityMode>("override");
  const [gitUserName, setGitUserName] = useState("");
  const [gitUserEmail, setGitUserEmail] = useState("");

  useEffect(() => {
    if (!isRemote) return;
    fetchLocalGitIdentity()
      .then((identity) => {
        const resolved: GitIdentityState = {
          userName: identity.user_name ?? "",
          userEmail: identity.user_email ?? "",
          detected: Boolean(identity.detected),
        };
        setLocalGitIdentity(resolved);
        if (resolved.detected) {
          setGitIdentityMode("local");
          setGitUserName(resolved.userName);
          setGitUserEmail(resolved.userEmail);
        } else {
          setGitIdentityMode("override");
        }
      })
      .catch(() => {});
  }, [isRemote]);

  return {
    localGitIdentity,
    gitIdentityMode,
    setGitIdentityMode,
    gitUserName,
    setGitUserName,
    gitUserEmail,
    setGitUserEmail,
  };
}

function useCreateProfileFormState(executorType: string) {
  const [name, setName] = useState("");
  const [mcpPolicy, setMcpPolicy] = useState("");
  const [prepareScript, setPrepareScript] = useState("");
  const [cleanupScript, setCleanupScript] = useState("");
  const { envVarRows, addEnvVar, removeEnvVar, updateEnvVar } = useEnvVarRows([]);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [spritesSecretId, setSpritesSecretId] = useState<string | null>(null);
  const remoteAuth = useCreateRemoteAuthState(executorType);
  const [dockerfile, setDockerfile] = useState("");
  const [imageTag, setImageTag] = useState("");
  const flags = useCreateRemoteFlags(executorType);
  const gitIdentity = useCreateGitIdentityState(flags.isRemote);
  const mcpPolicyError = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);

  useEffect(() => {
    listScriptPlaceholders()
      .then((res) => setPlaceholders(res.placeholders ?? []))
      .catch(() => {});
  }, []);

  useDefaultScripts(executorType, setPrepareScript);

  const buildEnvVars = useCallback((): ProfileEnvVar[] => {
    const vars = rowsToEnvVars(envVarRows).filter((ev) => ev.key !== SPRITES_TOKEN_KEY);
    if (flags.isSprites && spritesSecretId) {
      vars.push({ key: SPRITES_TOKEN_KEY, secret_id: spritesSecretId });
    }
    return vars;
  }, [envVarRows, flags.isSprites, spritesSecretId]);

  const prepareDesc = flags.isRemote
    ? "Runs inside the execution environment before the agent starts. Type {{ to see available placeholders."
    : "Runs on the host machine before the agent starts.";

  return {
    name,
    setName,
    mcpPolicy,
    setMcpPolicy,
    prepareScript,
    setPrepareScript,
    cleanupScript,
    setCleanupScript,
    envVarRows,
    addEnvVar,
    removeEnvVar,
    updateEnvVar,
    placeholders,
    spritesSecretId,
    setSpritesSecretId,
    networkPolicyRules: remoteAuth.networkPolicyRules,
    setNetworkPolicyRules: remoteAuth.setNetworkPolicyRules,
    remoteCredentials: remoteAuth.remoteCredentials,
    setRemoteCredentials: remoteAuth.setRemoteCredentials,
    agentEnvVars: remoteAuth.agentEnvVars,
    handleAgentEnvVarChange: remoteAuth.handleAgentEnvVarChange,
    localGitIdentity: gitIdentity.localGitIdentity,
    gitIdentityMode: gitIdentity.gitIdentityMode,
    setGitIdentityMode: gitIdentity.setGitIdentityMode,
    dockerfile,
    setDockerfile,
    imageTag,
    setImageTag,
    gitUserName: gitIdentity.gitUserName,
    setGitUserName: gitIdentity.setGitUserName,
    gitUserEmail: gitIdentity.gitUserEmail,
    setGitUserEmail: gitIdentity.setGitUserEmail,
    isRemote: flags.isRemote,
    isDocker: flags.isDocker,
    isSprites: flags.isSprites,
    mcpPolicyError,
    buildEnvVars,
    prepareDesc,
  };
}

function useCreateProfileSave(
  form: ReturnType<typeof useCreateProfileFormState>,
  executorId: string,
) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = useCallback(async () => {
    if (!form.name.trim() || form.mcpPolicyError) return;
    if (form.isSprites && !form.spritesSecretId) {
      setError("Sprites API key is required.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const profile = await createExecutorProfile(executorId, {
        name: form.name.trim(),
        mcp_policy: form.mcpPolicy || undefined,
        config: buildProfileConfig({
          isRemote: form.isRemote,
          isSprites: form.isSprites,
          networkPolicyRules: form.networkPolicyRules,
          remoteCredentials: form.remoteCredentials,
          agentEnvVars: form.agentEnvVars,
          gitIdentityMode: form.gitIdentityMode,
          localGitIdentity: form.localGitIdentity,
          gitUserName: form.gitUserName,
          gitUserEmail: form.gitUserEmail,
        }),
        prepare_script: form.prepareScript,
        cleanup_script: form.cleanupScript,
        env_vars: form.buildEnvVars(),
      });
      setExecutors(
        executors.map((e: Executor) =>
          e.id === executorId ? { ...e, profiles: [...(e.profiles ?? []), profile] } : e,
        ),
      );
      router.push(`/settings/executors/${profile.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create profile");
      setSaving(false);
    }
  }, [form, executorId, executors, setExecutors, router]);

  return { saving, error, handleSave };
}

function CreateProfileSections({
  executorType,
  form,
  secrets,
}: {
  executorType: string;
  form: ReturnType<typeof useCreateProfileFormState>;
  secrets: ReturnType<typeof useSecrets>["items"];
}) {
  return (
    <>
      <ProfileDetailsCard name={form.name} onNameChange={form.setName} />
      {form.isSprites && (
        <SpritesApiKeyCard
          secretId={form.spritesSecretId}
          onSecretIdChange={form.setSpritesSecretId}
          secrets={secrets}
        />
      )}
      {form.isDocker && (
        <DockerfileBuildCard
          dockerfile={form.dockerfile}
          onDockerfileChange={form.setDockerfile}
          imageTag={form.imageTag}
          onImageTagChange={form.setImageTag}
        />
      )}
      {form.isRemote && (
        <RemoteCredentialsCard
          isRemote={form.isRemote}
          selectedIds={form.remoteCredentials}
          onChange={form.setRemoteCredentials}
          agentEnvVars={form.agentEnvVars}
          onAgentEnvVarChange={form.handleAgentEnvVarChange}
          secrets={secrets}
          gitIdentityMode={form.gitIdentityMode}
          onGitIdentityModeChange={form.setGitIdentityMode}
          gitUserName={form.gitUserName}
          gitUserEmail={form.gitUserEmail}
          onGitUserNameChange={form.setGitUserName}
          onGitUserEmailChange={form.setGitUserEmail}
          localGitIdentity={form.localGitIdentity}
        />
      )}
      {form.isSprites && (
        <NetworkPoliciesCard
          rules={form.networkPolicyRules}
          onRulesChange={form.setNetworkPolicyRules}
        />
      )}
      <EnvVarsCard
        rows={form.envVarRows}
        secrets={secrets}
        onAdd={form.addEnvVar}
        onUpdate={form.updateEnvVar}
        onRemove={form.removeEnvVar}
      />
      <ScriptCard
        title="Prepare Script"
        description={form.prepareDesc}
        value={form.prepareScript}
        onChange={form.setPrepareScript}
        height="300px"
        placeholders={form.placeholders}
        executorType={executorType}
      />
      {form.isRemote && (
        <ScriptCard
          title="Cleanup Script"
          description="Runs after the agent session ends for cleanup tasks."
          value={form.cleanupScript}
          onChange={form.setCleanupScript}
          height="200px"
          placeholders={form.placeholders}
          executorType={executorType}
        />
      )}
      <McpPolicyCard
        mcpPolicy={form.mcpPolicy}
        mcpPolicyError={form.mcpPolicyError}
        onPolicyChange={form.setMcpPolicy}
      />
    </>
  );
}

function CreateProfileForm({
  executorType,
  typeInfo,
}: {
  executorType: string;
  typeInfo: { executorId: string; label: string; description: string };
}) {
  const { items: secrets } = useSecrets();
  const form = useCreateProfileFormState(executorType);
  const { saving, error, handleSave } = useCreateProfileSave(form, typeInfo.executorId);
  const spritesTokenMissing = form.isSprites && !form.spritesSecretId;

  return (
    <div className="space-y-8">
      <CreateProfileHeader
        type={executorType}
        label={typeInfo.label}
        description={typeInfo.description}
      />
      <CreateProfileSections executorType={executorType} form={form} secrets={secrets} />
      {spritesTokenMissing && (
        <p className="text-sm text-destructive">Sprites API key is required.</p>
      )}
      {error && <p className="text-sm text-destructive">{error}</p>}
      <CreateFormActions
        saving={saving}
        saveDisabled={
          !form.name.trim() || Boolean(form.mcpPolicyError) || spritesTokenMissing || saving
        }
        onSave={handleSave}
      />
    </div>
  );
}
