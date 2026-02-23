"use client";

import { use, useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import {
  updateExecutorProfile,
  deleteExecutorProfile,
  fetchLocalGitIdentity,
  listScriptPlaceholders,
} from "@/lib/api/domains/settings-api";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
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
import {
  type GitIdentityMode,
  type GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import { SpritesApiKeyCard } from "@/components/settings/profile-edit/sprites-api-key-card";
import {
  DockerSections,
  SpritesSections,
} from "@/components/settings/profile-edit/profile-runtime-sections";
import {
  ProfileHeader,
  ProfileFormActions,
  DeleteProfileDialog,
} from "@/components/settings/profile-edit/profile-edit-page-chrome";
import type { Executor, ExecutorProfile, ProfileEnvVar } from "@/lib/types/http";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";

const EXECUTORS_ROUTE = "/settings/executors";
const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";
function useProfileFromStore(profileId: string) {
  const executor = useAppStore(
    (state) =>
      state.executors.items.find((e: Executor) => e.profiles?.some((p) => p.id === profileId)) ??
      null,
  );
  const profile = executor?.profiles?.find((p: ExecutorProfile) => p.id === profileId) ?? null;
  return executor && profile ? { executor, profile } : null;
}

function deriveSpritesSecretId(envVars?: ProfileEnvVar[]): string | null {
  const row = envVars?.find((ev) => ev.key === SPRITES_TOKEN_KEY && ev.secret_id);
  return row?.secret_id ?? null;
}

function parseNetworkPolicyRules(config?: Record<string, string>): NetworkPolicyRule[] {
  const raw = config?.sprites_network_policy_rules;
  if (!raw) return [];
  try {
    return JSON.parse(raw) as NetworkPolicyRule[];
  } catch {
    return [];
  }
}

function parseRemoteCredentials(config?: Record<string, string>): string[] {
  const raw = config?.remote_credentials;
  if (!raw) return [];
  try {
    return JSON.parse(raw) as string[];
  } catch {
    return [];
  }
}

function useRemoteExecutorFlags(executorType: string) {
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

function useRemoteAuthState(profile: ExecutorProfile) {
  const [networkPolicyRules, setNetworkPolicyRules] = useState<NetworkPolicyRule[]>(() =>
    parseNetworkPolicyRules(profile.config),
  );
  const [remoteCredentials, setRemoteCredentials] = useState<string[]>(() =>
    parseRemoteCredentials(profile.config),
  );
  const [agentEnvVars, setAgentEnvVars] = useState<Record<string, string | null>>(() => {
    const raw = profile.config?.remote_auth_secrets;
    if (!raw) return {};
    try {
      return JSON.parse(raw) as Record<string, string | null>;
    } catch {
      return {};
    }
  });

  const handleAgentEnvVarChange = useCallback((agentId: string, secretId: string | null) => {
    setAgentEnvVars((prev) => ({ ...prev, [agentId]: secretId }));
  }, []);

  return {
    networkPolicyRules,
    setNetworkPolicyRules,
    remoteCredentials,
    setRemoteCredentials,
    agentEnvVars,
    handleAgentEnvVarChange,
  };
}

function useGitIdentityState(isRemote: boolean, profile: ExecutorProfile) {
  const [localGitIdentity, setLocalGitIdentity] = useState<GitIdentityState>({
    userName: "",
    userEmail: "",
    detected: false,
  });
  const [gitIdentityMode, setGitIdentityMode] = useState<GitIdentityMode>("override");
  const [gitUserName, setGitUserName] = useState(profile.config?.git_user_name ?? "");
  const [gitUserEmail, setGitUserEmail] = useState(profile.config?.git_user_email ?? "");

  useEffect(() => {
    if (!isRemote) return;
    fetchLocalGitIdentity()
      .then((identity) => {
        const local: GitIdentityState = {
          userName: identity.user_name ?? "",
          userEmail: identity.user_email ?? "",
          detected: Boolean(identity.detected),
        };
        setLocalGitIdentity(local);

        const hasStoredOverride = Boolean(
          profile.config?.git_user_name?.trim() || profile.config?.git_user_email?.trim(),
        );
        if (hasStoredOverride) {
          setGitIdentityMode("override");
          return;
        }
        if (local.detected) {
          setGitIdentityMode("local");
          setGitUserName(local.userName);
          setGitUserEmail(local.userEmail);
          return;
        }
        setGitIdentityMode("override");
      })
      .catch(() => {});
  }, [isRemote, profile.config?.git_user_email, profile.config?.git_user_name]);

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

export default function ProfileEditPage({ params }: { params: Promise<{ profileId: string }> }) {
  const { profileId } = use(params);
  const router = useRouter();
  const result = useProfileFromStore(profileId);

  if (!result) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">Profile not found</p>
          <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
            Back to Executors
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <ProfileEditForm key={result.profile.id} executor={result.executor} profile={result.profile} />
  );
}

function useProfilePersistence(executor: Executor, profile: ExecutorProfile) {
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const save = useCallback(
    async (data: {
      name: string;
      mcp_policy?: string;
      config?: Record<string, string>;
      prepare_script: string;
      cleanup_script: string;
      env_vars: ProfileEnvVar[];
    }) => {
      setSaving(true);
      setError(null);
      try {
        const updated = await updateExecutorProfile(executor.id, profile.id, data);
        setExecutors(
          executors.map((e: Executor) =>
            e.id === executor.id
              ? { ...e, profiles: e.profiles?.map((p) => (p.id === updated.id ? updated : p)) }
              : e,
          ),
        );
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to save profile");
      } finally {
        setSaving(false);
      }
    },
    [executor.id, profile.id, executors, setExecutors],
  );

  const remove = useCallback(async () => {
    setDeleting(true);
    try {
      await deleteExecutorProfile(executor.id, profile.id);
      setExecutors(
        executors.map((e: Executor) =>
          e.id === executor.id
            ? { ...e, profiles: e.profiles?.filter((p) => p.id !== profile.id) }
            : e,
        ),
      );
      router.push(EXECUTORS_ROUTE);
    } catch {
      setDeleting(false);
      setDeleteDialogOpen(false);
    }
  }, [executor.id, profile.id, executors, setExecutors, router]);

  return { saving, error, deleting, deleteDialogOpen, setDeleteDialogOpen, save, remove };
}

function useProfileFormState(executor: Executor, profile: ExecutorProfile) {
  const [name, setName] = useState(profile.name);
  const [mcpPolicy, setMcpPolicy] = useState(profile.mcp_policy ?? "");
  const [prepareScript, setPrepareScript] = useState(profile.prepare_script ?? "");
  const [cleanupScript, setCleanupScript] = useState(profile.cleanup_script ?? "");
  const { envVarRows, addEnvVar, removeEnvVar, updateEnvVar } = useEnvVarRows(profile.env_vars);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [spritesSecretId, setSpritesSecretId] = useState<string | null>(() =>
    deriveSpritesSecretId(profile.env_vars),
  );
  const flags = useRemoteExecutorFlags(executor.type);
  const remoteAuth = useRemoteAuthState(profile);
  const gitIdentity = useGitIdentityState(flags.isRemote, profile);
  const mcpPolicyError = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);

  useEffect(() => {
    listScriptPlaceholders()
      .then((res) => setPlaceholders(res.placeholders ?? []))
      .catch(() => {});
  }, []);

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

function buildSaveConfig(
  form: ReturnType<typeof useProfileFormState>,
  baseConfig?: Record<string, string>,
): Record<string, string> {
  const config: Record<string, string> = { ...baseConfig };
  if (form.isSprites && form.networkPolicyRules.length > 0) {
    config.sprites_network_policy_rules = JSON.stringify(form.networkPolicyRules);
  } else {
    delete config.sprites_network_policy_rules;
  }
  if (form.isSprites && form.remoteCredentials.length > 0) {
    config.remote_credentials = JSON.stringify(form.remoteCredentials);
  } else {
    delete config.remote_credentials;
  }
  const nonNullEnvVars = Object.fromEntries(
    Object.entries(form.agentEnvVars).filter(([, v]) => v != null),
  );
  if (form.isSprites && Object.keys(nonNullEnvVars).length > 0) {
    config.remote_auth_secrets = JSON.stringify(nonNullEnvVars);
  } else {
    delete config.remote_auth_secrets;
  }
  const effectiveName =
    form.gitIdentityMode === "local"
      ? form.localGitIdentity.userName.trim()
      : form.gitUserName.trim();
  const effectiveEmail =
    form.gitIdentityMode === "local"
      ? form.localGitIdentity.userEmail.trim()
      : form.gitUserEmail.trim();
  if (form.isRemote && effectiveName) {
    config.git_user_name = effectiveName;
  } else {
    delete config.git_user_name;
  }
  if (form.isRemote && effectiveEmail) {
    config.git_user_email = effectiveEmail;
  } else {
    delete config.git_user_email;
  }
  return config;
}

function ProfileEditSections({
  executor,
  profile,
  form,
  secrets,
}: {
  executor: Executor;
  profile: ExecutorProfile;
  form: ReturnType<typeof useProfileFormState>;
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
      {form.isDocker && <DockerSections profile={profile} />}
      {form.isRemote && (
        <SpritesSections
          isRemote={form.isRemote}
          isSprites={form.isSprites}
          secretId={form.spritesSecretId}
          networkRules={form.networkPolicyRules}
          onNetworkRulesChange={form.setNetworkPolicyRules}
          remoteCredentials={form.remoteCredentials}
          onRemoteCredentialsChange={form.setRemoteCredentials}
          agentEnvVars={form.agentEnvVars}
          onAgentEnvVarChange={form.handleAgentEnvVarChange}
          gitIdentityMode={form.gitIdentityMode}
          onGitIdentityModeChange={form.setGitIdentityMode}
          gitUserName={form.gitUserName}
          gitUserEmail={form.gitUserEmail}
          onGitUserNameChange={form.setGitUserName}
          onGitUserEmailChange={form.setGitUserEmail}
          localGitIdentity={form.localGitIdentity}
          secrets={secrets}
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
        executorType={executor.type}
      />
      {form.isRemote && (
        <ScriptCard
          title="Cleanup Script"
          description="Runs after the agent session ends for cleanup tasks."
          value={form.cleanupScript}
          onChange={form.setCleanupScript}
          height="200px"
          placeholders={form.placeholders}
          executorType={executor.type}
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

function ProfileEditForm({ executor, profile }: { executor: Executor; profile: ExecutorProfile }) {
  const { items: secrets } = useSecrets();
  const persistence = useProfilePersistence(executor, profile);
  const form = useProfileFormState(executor, profile);
  const spritesTokenMissing = form.isSprites && !form.spritesSecretId;

  const handleSave = () => {
    if (!form.name.trim() || form.mcpPolicyError || spritesTokenMissing) return;
    void persistence.save({
      name: form.name.trim(),
      mcp_policy: form.mcpPolicy || undefined,
      config: buildSaveConfig(form, profile.config),
      prepare_script: form.prepareScript,
      cleanup_script: form.cleanupScript,
      env_vars: form.buildEnvVars(),
    });
  };

  return (
    <div className="space-y-8">
      <ProfileHeader
        executor={executor}
        profileName={profile.name}
        description={getExecutorDescription(executor.type)}
      />
      <ProfileEditSections executor={executor} profile={profile} form={form} secrets={secrets} />
      {spritesTokenMissing && (
        <p className="text-sm text-destructive">Sprites API key is required.</p>
      )}
      {persistence.error && <p className="text-sm text-destructive">{persistence.error}</p>}
      <ProfileFormActions
        saving={persistence.saving}
        saveDisabled={
          !form.name.trim() ||
          Boolean(form.mcpPolicyError) ||
          spritesTokenMissing ||
          persistence.saving
        }
        onSave={handleSave}
        onDelete={() => persistence.setDeleteDialogOpen(true)}
      />
      <DeleteProfileDialog
        open={persistence.deleteDialogOpen}
        onOpenChange={persistence.setDeleteDialogOpen}
        onDelete={persistence.remove}
        deleting={persistence.deleting}
      />
    </div>
  );
}

function getExecutorDescription(type: string): string {
  if (type === "local") return "Runs agents directly in the repository folder.";
  if (type === "worktree") return "Creates git worktrees for isolated agent sessions.";
  if (type === "local_docker") return "Runs Docker containers on this machine.";
  if (type === "remote_docker") return "Connects to a remote Docker host.";
  if (type === "sprites") return "Runs agents in Sprites.dev cloud sandboxes.";
  return "Custom executor.";
}
