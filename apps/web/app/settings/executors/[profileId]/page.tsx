"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import {
  updateExecutorProfile,
  deleteExecutorProfile,
  removeDockerContainer,
  fetchLocalGitIdentity,
  listScriptPlaceholders,
} from "@/lib/api/domains/settings-api";
import type { ScriptPlaceholder } from "@/lib/api/domains/settings-api";
import { ProfileDetailsCard } from "@/components/settings/profile-edit/profile-details-card";
import { getExecutorDescription } from "@/components/settings/executor-description";
import {
  McpPolicyCard,
  validateMcpPolicy,
} from "@/components/settings/profile-edit/mcp-policy-card";
import {
  EnvVarsCard,
  useEnvVarRows,
  envVarsToRows,
} from "@/components/settings/profile-edit/env-vars-card";
import { ProfileScriptCards } from "@/components/settings/profile-edit/profile-script-cards";
import { SSHAgentReadinessCard } from "@/components/settings/ssh-agent-readiness-card";
import { SSHTaskDirReclamationCard } from "@/components/settings/ssh-task-dir-reclamation-card";
import {
  type GitIdentityMode,
  type GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import { SpritesApiKeyCard } from "@/components/settings/profile-edit/sprites-api-key-card";
import {
  DockerSections,
  SpritesSections,
} from "@/components/settings/profile-edit/profile-runtime-sections";
import { useDockerProfileContainers } from "@/components/settings/profile-edit/docker-sections";
import {
  ProfileHeader,
  ProfileFormActions,
  DeleteProfileDialog,
  upsertExecutorProfile,
  type SaveStatus,
} from "@/components/settings/profile-edit/profile-edit-page-chrome";
import { ProfileConnectionSettingsAction } from "@/components/settings/profile-edit/profile-connection-settings-action";
import type { ExecutorProfileSavePayload } from "@/components/settings/profile-edit/use-executor-profile-save-contributor";
import {
  useKubernetesProfileConnection,
  useProfileSettingsSave,
} from "@/components/settings/profile-edit/use-profile-settings-save";
import {
  buildProfileEnvVars,
  profileSaveInvalidReason,
} from "@/components/settings/profile-edit/profile-edit-form-helpers";
import { useToast } from "@/components/toast-provider";
import {
  deriveSpritesSecretId,
  getGitIdentityBaseline,
  parseAgentConfigBundles,
  parseNetworkPolicyRules,
  parseRemoteAuthSecrets,
  parseRemoteCredentials,
} from "@/components/settings/profile-edit/executor-profile-baselines";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import { executorProfileDiscoveryTarget } from "@/lib/settings-discovery/dynamic-targets";
import { buildSaveConfig } from "@/components/settings/profile-edit/serialize-executor-config";
import { useProfileRuntimeFormState } from "@/components/settings/profile-edit/use-profile-runtime-form-state";
import { KubernetesProfileSections } from "@/components/settings/kubernetes-profile-sections";
import { KubernetesReadOnlyNotice } from "@/components/settings/kubernetes-read-only-notice";
import { KubernetesProfileClusterSection } from "@/components/settings/kubernetes-profile-cluster-section";
import {
  parseKubernetesProfileConfig,
  replaceKubernetesProfileConfig,
} from "@/components/settings/kubernetes-config";
import { kubernetesExecutorInvalidReason } from "@/components/settings/kubernetes-validation";
import {
  useKubernetesAdminAccess,
  useKubernetesDiagnostics,
  useKubernetesSessions,
} from "@/hooks/domains/settings/use-kubernetes-settings";

const EXECUTORS_ROUTE = "/settings/executors";

function useProfileFromStore(profileId: string) {
  const executor = useAppStore(
    (state) =>
      state.executors.items.find((e: Executor) => e.profiles?.some((p) => p.id === profileId)) ??
      null,
  );
  const profile = executor?.profiles?.find((p: ExecutorProfile) => p.id === profileId) ?? null;
  return executor && profile ? { executor, profile } : null;
}

function useRemoteAuthState(profile: ExecutorProfile) {
  const [networkPolicyRules, setNetworkPolicyRules] = useState<NetworkPolicyRule[]>(() =>
    parseNetworkPolicyRules(profile.config),
  );
  const [remoteCredentials, setRemoteCredentials] = useState<string[]>(() =>
    parseRemoteCredentials(profile.config),
  );
  const [configBundleIds, setConfigBundleIds] = useState<string[]>(() =>
    parseAgentConfigBundles(profile.config),
  );
  const [agentEnvVars, setAgentEnvVars] = useState<Record<string, string | null>>(() =>
    parseRemoteAuthSecrets(profile.config),
  );

  const handleAgentEnvVarChange = useCallback((agentId: string, secretId: string | null) => {
    setAgentEnvVars((prev) => ({ ...prev, [agentId]: secretId }));
  }, []);

  const reset = useCallback(() => {
    setNetworkPolicyRules(parseNetworkPolicyRules(profile.config));
    setRemoteCredentials(parseRemoteCredentials(profile.config));
    setConfigBundleIds(parseAgentConfigBundles(profile.config));
    setAgentEnvVars(parseRemoteAuthSecrets(profile.config));
  }, [profile.config]);

  return {
    networkPolicyRules,
    setNetworkPolicyRules,
    remoteCredentials,
    setRemoteCredentials,
    configBundleIds,
    setConfigBundleIds,
    agentEnvVars,
    handleAgentEnvVarChange,
    reset,
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
  const [loaded, setLoaded] = useState(!isRemote);

  useEffect(() => {
    if (!isRemote) {
      setLoaded(true);
      return;
    }
    setLoaded(false);
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
      .catch(() => {})
      .finally(() => setLoaded(true));
  }, [isRemote, profile.config?.git_user_email, profile.config?.git_user_name]);

  const reset = useCallback(() => {
    const baseline = getGitIdentityBaseline(profile, localGitIdentity);
    setGitIdentityMode(baseline.mode);
    setGitUserName(baseline.userName);
    setGitUserEmail(baseline.userEmail);
  }, [localGitIdentity, profile]);

  return {
    localGitIdentity,
    gitIdentityMode,
    setGitIdentityMode,
    gitUserName,
    setGitUserName,
    gitUserEmail,
    setGitUserEmail,
    loaded,
    reset,
  };
}

export default function ProfileEditPage({ profileId }: { profileId: string }) {
  const { t } = useTranslation();
  const router = useRouter();
  const result = useProfileFromStore(profileId);

  if (!result) {
    return (
      <Card>
        <CardContent className="py-12 text-center">
          <p className="text-muted-foreground">{t("executors:profileNotFound")}</p>
          <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
            {t("executors:backToExecutors")}
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
  const { t } = useTranslation();
  const router = useRouter();
  const { toast } = useToast();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const save = useCallback(
    async (data: ExecutorProfileSavePayload) => {
      setSaveStatus("loading");
      setError(null);
      try {
        const updated = await updateExecutorProfile(executor.id, profile.id, data);
        setSaveStatus("success");
        toast({ title: t("executors:profileSaved"), variant: "success" });
        setExecutors(upsertExecutorProfile(executors, executor, updated));
        window.setTimeout(() => setSaveStatus("idle"), 1500);
      } catch (err) {
        const message = err instanceof Error ? err.message : t("executors:failedToSaveProfile");
        setError(message);
        setSaveStatus("error");
        toast({
          title: t("executors:failedToSaveProfile"),
          description: message,
          variant: "error",
        });
        throw err;
      }
    },
    [executor, profile.id, executors, setExecutors, toast, t],
  );

  const remove = useCallback(
    async (beforeDelete?: () => Promise<void>) => {
      setDeleting(true);
      try {
        await beforeDelete?.();
        await deleteExecutorProfile(executor.id, profile.id);
        setExecutors(
          executors.map((e: Executor) =>
            e.id === executor.id
              ? { ...e, profiles: e.profiles?.filter((p) => p.id !== profile.id) }
              : e,
          ),
        );
        runWithNavigationBlockerBypassed(() => router.push(EXECUTORS_ROUTE));
      } catch {
        setDeleting(false);
        setDeleteDialogOpen(false);
      }
    },
    [executor.id, profile.id, executors, setExecutors, router],
  );

  return { saveStatus, error, deleting, deleteDialogOpen, setDeleteDialogOpen, save, remove };
}

export function useProfileFormState(executor: Executor, profile: ExecutorProfile) {
  const { t } = useTranslation();
  const [name, setName] = useState(profile.name);
  const [mcpPolicy, setMcpPolicy] = useState(profile.mcp_policy ?? "");
  const [prepareScript, setPrepareScript] = useState(profile.prepare_script ?? "");
  const [cleanupScript, setCleanupScript] = useState(profile.cleanup_script ?? "");
  const runtime = useProfileRuntimeFormState(executor, profile);
  const { envVarRows, addEnvVar, removeEnvVar, updateEnvVar, resetEnvVars } = useEnvVarRows(
    profile.env_vars,
  );
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [spritesSecretId, setSpritesSecretId] = useState<string | null>(() =>
    deriveSpritesSecretId(profile.env_vars),
  );
  const remoteAuth = useRemoteAuthState(profile);
  const gitIdentity = useGitIdentityState(runtime.isRemote, profile);
  const mcpPolicyErrorKey = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);

  useEffect(() => {
    listScriptPlaceholders()
      .then((res) => setPlaceholders(res.placeholders ?? []))
      .catch(() => {});
  }, []);

  const buildEnvVars = useCallback(
    () => buildProfileEnvVars(envVarRows, runtime.isSprites, spritesSecretId),
    [envVarRows, runtime.isSprites, spritesSecretId],
  );

  const prepareDesc = runtime.isRemote
    ? t("executors:prepareScriptDescriptionRemote", { trigger: "{{" })
    : t("executors:prepareScriptDescriptionLocal");

  const reset = useCallback(() => {
    setName(profile.name);
    setMcpPolicy(profile.mcp_policy ?? "");
    setPrepareScript(profile.prepare_script ?? "");
    setCleanupScript(profile.cleanup_script ?? "");
    runtime.resetRuntime();
    resetEnvVars(profile.env_vars);
    setSpritesSecretId(deriveSpritesSecretId(profile.env_vars));
    remoteAuth.reset();
    gitIdentity.reset();
  }, [gitIdentity, profile, remoteAuth, resetEnvVars, runtime]);

  return {
    ...runtime,
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
    configBundleIds: remoteAuth.configBundleIds,
    setConfigBundleIds: remoteAuth.setConfigBundleIds,
    agentEnvVars: remoteAuth.agentEnvVars,
    handleAgentEnvVarChange: remoteAuth.handleAgentEnvVarChange,
    localGitIdentity: gitIdentity.localGitIdentity,
    gitIdentityLoaded: gitIdentity.loaded,
    gitIdentityMode: gitIdentity.gitIdentityMode,
    setGitIdentityMode: gitIdentity.setGitIdentityMode,
    gitUserName: gitIdentity.gitUserName,
    setGitUserName: gitIdentity.setGitUserName,
    gitUserEmail: gitIdentity.gitUserEmail,
    setGitUserEmail: gitIdentity.setGitUserEmail,
    mcpPolicyErrorKey,
    buildEnvVars,
    prepareDesc,
    reset,
  };
}

type ProfileEditSectionsProps = {
  executor: Executor;
  profile: ExecutorProfile;
  form: ReturnType<typeof useProfileFormState>;
  secrets: ReturnType<typeof useSecrets>["items"];
};

function ExecutorSpecificSections({ executor, profile, form, secrets }: ProfileEditSectionsProps) {
  const gitIdentityBaseline = getGitIdentityBaseline(profile, form.localGitIdentity);
  const canManageKubernetes = useKubernetesAdminAccess();
  return (
    <>
      {executor.type === "ssh" && (
        <SSHAgentReadinessCard
          executorId={executor.id}
          shell={form.sshShell}
          baselineShell={profile.config?.ssh_shell ?? ""}
          onShellChange={form.setSshShell}
        />
      )}
      {executor.type === "ssh" && (
        <SSHTaskDirReclamationCard
          executor={executor}
          profile={profile}
          enabled={form.sshReclaimTaskDir}
          onEnabledChange={form.setSshReclaimTaskDir}
        />
      )}
      {form.isSprites && (
        <SpritesApiKeyCard
          secretId={form.spritesSecretId}
          baselineSecretId={deriveSpritesSecretId(profile.env_vars)}
          onSecretIdChange={form.setSpritesSecretId}
          secrets={secrets}
        />
      )}
      {form.isDocker && (
        <DockerSections
          profile={profile}
          dockerfile={form.dockerfile}
          onDockerfileChange={form.setDockerfile}
          imageTag={form.imageTag}
          onImageTagChange={form.setImageTag}
        />
      )}
      {form.isKubernetes && (
        <KubernetesProfileSections
          form={form.kubernetesProfile}
          baseline={parseKubernetesProfileConfig(profile.config)}
          onChange={form.setKubernetesProfile}
          canManage={canManageKubernetes}
        />
      )}
      {form.isRemote && (
        <SpritesSections
          isRemote={form.isRemote}
          isSprites={form.isSprites}
          secretId={form.spritesSecretId}
          networkRules={form.networkPolicyRules}
          baselineNetworkRules={parseNetworkPolicyRules(profile.config)}
          onNetworkRulesChange={form.setNetworkPolicyRules}
          remoteCredentials={form.remoteCredentials}
          baselineRemoteCredentials={parseRemoteCredentials(profile.config)}
          onRemoteCredentialsChange={form.setRemoteCredentials}
          configBundleIds={form.configBundleIds}
          baselineConfigBundleIds={parseAgentConfigBundles(profile.config)}
          onConfigBundleChange={form.setConfigBundleIds}
          isSSH={form.isSSH}
          agentEnvVars={form.agentEnvVars}
          baselineAgentEnvVars={parseRemoteAuthSecrets(profile.config)}
          onAgentEnvVarChange={form.handleAgentEnvVarChange}
          gitIdentityMode={form.gitIdentityMode}
          baselineGitIdentityMode={gitIdentityBaseline.mode}
          onGitIdentityModeChange={form.setGitIdentityMode}
          gitUserName={form.gitUserName}
          gitUserEmail={form.gitUserEmail}
          baselineGitUserName={gitIdentityBaseline.userName}
          baselineGitUserEmail={gitIdentityBaseline.userEmail}
          onGitUserNameChange={form.setGitUserName}
          onGitUserEmailChange={form.setGitUserEmail}
          localGitIdentity={form.localGitIdentity}
          secrets={secrets}
        />
      )}
    </>
  );
}

function ProfileEditSections({ executor, profile, form, secrets }: ProfileEditSectionsProps) {
  return (
    <>
      <ProfileDetailsCard
        name={form.name}
        baselineName={profile.name}
        onNameChange={form.setName}
        discoveryTargetId={executorProfileDiscoveryTarget(profile.id, "profile-details")}
      />
      <ExecutorSpecificSections
        executor={executor}
        profile={profile}
        form={form}
        secrets={secrets}
      />
      <EnvVarsCard
        rows={form.envVarRows}
        baselineRows={envVarsToRows(profile.env_vars)}
        secrets={secrets}
        onAdd={form.addEnvVar}
        onUpdate={form.updateEnvVar}
        onRemove={form.removeEnvVar}
        discoveryTargetId={executorProfileDiscoveryTarget(profile.id, "environment-variables")}
      />
      <ProfileScriptCards
        executorType={executor.type}
        profileId={profile.id}
        scripts={{
          isRemote: form.isRemote,
          prepareDescription: form.prepareDesc,
          prepareValue: form.prepareScript,
          prepareBaseline: profile.prepare_script ?? "",
          onPrepareChange: form.setPrepareScript,
          cleanupValue: form.cleanupScript,
          cleanupBaseline: profile.cleanup_script ?? "",
          onCleanupChange: form.setCleanupScript,
          placeholders: form.placeholders,
        }}
      />
      <McpPolicyCard
        mcpPolicy={form.mcpPolicy}
        baselinePolicy={profile.mcp_policy ?? ""}
        mcpPolicyErrorKey={form.mcpPolicyErrorKey}
        onPolicyChange={form.setMcpPolicy}
        discoveryTargetId={executorProfileDiscoveryTarget(profile.id, "mcp-policy")}
      />
    </>
  );
}

function ProfileEditForm({ executor, profile }: { executor: Executor; profile: ExecutorProfile }) {
  const { t } = useTranslation();
  const { items: secrets } = useSecrets();
  const persistence = useProfilePersistence(executor, profile);
  const form = useProfileFormState(executor, profile);
  const canManageKubernetes = useKubernetesAdminAccess();
  const connection = useKubernetesProfileConnection(executor, form.isKubernetes);
  const diagnostics = useKubernetesDiagnostics();
  const sessions = useKubernetesSessions(executor.id, form.isKubernetes);
  const relatedContainers = useDockerProfileContainers(profile.id, form.isDocker);
  const spritesTokenMissing = form.isSprites && !form.spritesSecretId;
  const memberReadOnly = form.isKubernetes && !canManageKubernetes;
  const sharedConfig = buildSaveConfig(form, profile.config);
  const savePayload = {
    name: form.name.trim(),
    mcp_policy: form.mcpPolicy || undefined,
    config: form.isKubernetes
      ? replaceKubernetesProfileConfig(sharedConfig, form.kubernetesProfile)
      : sharedConfig,
    prepare_script: form.prepareScript,
    cleanup_script: form.cleanupScript,
    env_vars: form.buildEnvVars(),
  };
  const profileInvalidReason = profileSaveInvalidReason(form, canManageKubernetes, t);
  const connectionInvalidReason = form.isKubernetes
    ? kubernetesExecutorInvalidReason(connection.form, canManageKubernetes, t)
    : undefined;
  const invalidReason = connectionInvalidReason ?? profileInvalidReason;
  const baselineReady = useProfileSettingsSave({
    executorId: executor.id,
    profileId: profile.id,
    payload: savePayload,
    isKubernetes: form.isKubernetes,
    isRemote: form.isRemote,
    gitIdentityLoaded: form.gitIdentityLoaded,
    canManageKubernetes,
    invalidReason,
    saveProfile: persistence.save,
    discardProfile: form.reset,
    clearDiagnostics: diagnostics.clear,
    connection,
  });

  const handleDelete = (options?: { removeRelatedDockerContainers?: boolean }) => {
    const beforeDelete = options?.removeRelatedDockerContainers
      ? async () => {
          await Promise.all(
            relatedContainers.containers.map((container) => removeDockerContainer(container.id)),
          );
          await relatedContainers.refresh();
        }
      : undefined;
    void persistence.remove(beforeDelete);
  };

  return (
    <div className="min-w-0 space-y-8 overflow-x-clip">
      <ProfileHeader
        executor={executor}
        profileName={profile.name}
        description={getExecutorDescription(executor.type)}
        actions={<ProfileConnectionSettingsAction executor={executor} />}
      />
      {memberReadOnly && <KubernetesReadOnlyNotice />}
      {form.isKubernetes && (
        <KubernetesProfileClusterSection
          executor={executor}
          form={form.kubernetesProfile}
          connectionForm={connection.form}
          connectionBaseline={connection.baseline}
          onConnectionChange={connection.setForm}
          canManage={canManageKubernetes}
          diagnosticsState={diagnostics}
          sessionsState={sessions}
        />
      )}
      <fieldset
        disabled={!baselineReady || persistence.saveStatus === "loading" || memberReadOnly}
        className="min-w-0 space-y-8"
      >
        <ProfileEditSections executor={executor} profile={profile} form={form} secrets={secrets} />
      </fieldset>
      {spritesTokenMissing && (
        <p className="text-sm text-destructive">{t("executors:spritesApiKeyIsRequired")}</p>
      )}
      {persistence.error && <p className="text-sm text-destructive">{persistence.error}</p>}
      <ProfileFormActions
        onDelete={() => persistence.setDeleteDialogOpen(true)}
        disabled={memberReadOnly}
      />
      <DeleteProfileDialog
        open={persistence.deleteDialogOpen}
        onOpenChange={persistence.setDeleteDialogOpen}
        onDelete={handleDelete}
        deleting={persistence.deleting}
        relatedDockerContainerCount={form.isDocker ? relatedContainers.containers.length : 0}
      />
    </div>
  );
}
