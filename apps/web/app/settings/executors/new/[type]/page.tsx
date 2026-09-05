"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
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
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { serializeSettingsRevision } from "@/components/settings/settings-save-revision";
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
  DockerfileBuildCard,
  type DockerBuildSuccess,
} from "@/components/settings/profile-edit/docker-sections";
import { SpritesApiKeyCard } from "@/components/settings/profile-edit/sprites-api-key-card";
import { NetworkPoliciesCard } from "@/components/settings/profile-edit/sprites-sections";
import {
  RemoteCredentialsCard,
  type GitIdentityMode,
  type GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import type { Executor, ExecutorType, ProfileEnvVar } from "@/lib/types/http";

import { EXECUTOR_TYPE_MAP, executorTypeLabel, type ExecutorTypeInfo } from "./executor-types";
import { SSHCreatePage } from "./ssh-create-page";
import { KubernetesCreatePage } from "./kubernetes-create-page";

const EXECUTORS_ROUTE = "/settings/executors";
const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";

const DefaultIcon = EXECUTOR_ICON_MAP.local;

function ExecutorTypeIcon({ type }: { type: string }) {
  const Icon = EXECUTOR_ICON_MAP[type] ?? DefaultIcon;
  return <Icon className="h-5 w-5 text-muted-foreground" />;
}

export default function CreateProfilePage({ executorType }: { executorType: string }) {
  const typeInfo = EXECUTOR_TYPE_MAP[executorType];

  if (!typeInfo) {
    return <InvalidTypeFallback />;
  }

  if (executorType === "ssh") {
    return <SSHCreatePage />;
  }

  if (executorType === "k8s") {
    return <KubernetesCreatePage />;
  }

  return <CreateProfileForm executorType={executorType as ExecutorType} typeInfo={typeInfo} />;
}

function InvalidTypeFallback() {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <Card>
      <CardContent className="py-12 text-center">
        <p className="text-muted-foreground">{t("executors:unknownExecutorType")}</p>
        <Button className="mt-4 cursor-pointer" onClick={() => router.push(EXECUTORS_ROUTE)}>
          {t("executors:backToExecutors")}
        </Button>
      </CardContent>
    </Card>
  );
}

function CreateProfileHeader({ type, typeInfo }: { type: string; typeInfo: ExecutorTypeInfo }) {
  const { t } = useTranslation();
  const router = useRouter();
  return (
    <>
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <ExecutorTypeIcon type={type} />
            <h2 className="min-w-0 break-words text-2xl font-bold">
              {t("executors:newTypeProfile", { type: executorTypeLabel(typeInfo, t) })}
            </h2>
            <Badge variant="outline" className="text-[10px]">
              {getExecutorLabel(type)}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{t(typeInfo.descriptionKey)}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push(EXECUTORS_ROUTE)}
          className="min-h-11 w-full cursor-pointer text-sm md:min-h-7 md:w-auto md:text-xs"
        >
          {t("executors:backToExecutors")}
        </Button>
      </div>
      <Separator />
    </>
  );
}

type BuildProfileConfigInput = {
  isRemote: boolean;
  isSprites: boolean;
  isDocker: boolean;
  networkPolicyRules: NetworkPolicyRule[];
  remoteCredentials: string[];
  configBundleIds: string[];
  agentEnvVars: Record<string, string | null>;
  gitIdentityMode: GitIdentityMode;
  localGitIdentity: GitIdentityState;
  gitUserName: string;
  gitUserEmail: string;
  dockerfile: string;
  imageTag: string;
};

function buildProfileConfig(input: BuildProfileConfigInput): Record<string, string> | undefined {
  const {
    isRemote,
    isSprites,
    isDocker,
    networkPolicyRules,
    remoteCredentials,
    configBundleIds,
    agentEnvVars,
    gitIdentityMode,
    localGitIdentity,
    gitUserName,
    gitUserEmail,
    dockerfile,
    imageTag,
  } = input;
  const config: Record<string, string> = {};
  if (isSprites && networkPolicyRules.length > 0) {
    config.sprites_network_policy_rules = JSON.stringify(networkPolicyRules);
  }
  if (isRemote && remoteCredentials.length > 0) {
    config.remote_credentials = JSON.stringify(remoteCredentials);
  }
  if (isRemote && configBundleIds.length > 0) {
    config.agent_config_bundles = JSON.stringify(configBundleIds);
  }
  const nonNullEnvVars = Object.fromEntries(
    Object.entries(agentEnvVars).filter(([, v]) => v != null),
  );
  if (isRemote && Object.keys(nonNullEnvVars).length > 0) {
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
  applyDockerCreateConfig(config, isDocker, dockerfile, imageTag);
  return Object.keys(config).length > 0 ? config : undefined;
}

function applyDockerCreateConfig(
  config: Record<string, string>,
  isDocker: boolean,
  dockerfile: string,
  imageTag: string,
): void {
  if (!isDocker) return;
  if (dockerfile.trim()) {
    config.dockerfile = dockerfile;
  }
  if (imageTag.trim()) {
    config.image_tag = imageTag.trim();
  }
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

function useCreateRemoteFlags(executorType: ExecutorType) {
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

function useCreateRemoteAuthState() {
  const [remoteCredentials, setRemoteCredentials] = useState<string[]>([]);
  const [configBundleIds, setConfigBundleIds] = useState<string[]>([]);
  const [agentEnvVars, setAgentEnvVars] = useState<Record<string, string | null>>({});
  const [networkPolicyRules, setNetworkPolicyRules] = useState<NetworkPolicyRule[]>([]);

  const handleAgentEnvVarChange = useCallback((agentId: string, secretId: string | null) => {
    setAgentEnvVars((prev) => ({ ...prev, [agentId]: secretId }));
  }, []);

  return {
    remoteCredentials,
    setRemoteCredentials,
    configBundleIds,
    setConfigBundleIds,
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

function useCreateProfileFormState(executorType: ExecutorType) {
  const { t } = useTranslation();
  const [name, setName] = useState(() => (executorType === "local_docker" ? "Docker" : ""));
  const [mcpPolicy, setMcpPolicy] = useState("");
  const [prepareScript, setPrepareScript] = useState("");
  const [cleanupScript, setCleanupScript] = useState("");
  const { envVarRows, addEnvVar, removeEnvVar, updateEnvVar } = useEnvVarRows([]);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [spritesSecretId, setSpritesSecretId] = useState<string | null>(null);
  const remoteAuth = useCreateRemoteAuthState();
  const [dockerfile, setDockerfile] = useState("");
  const [imageTag, setImageTag] = useState("");
  const [builtDockerImage, setBuiltDockerImage] = useState<DockerBuildSuccess | null>(null);
  const flags = useCreateRemoteFlags(executorType);
  const gitIdentity = useCreateGitIdentityState(flags.isRemote);
  const mcpPolicyErrorKey = useMemo(() => validateMcpPolicy(mcpPolicy), [mcpPolicy]);

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

  const recordDockerBuildSuccess = useCallback((result: DockerBuildSuccess) => {
    setBuiltDockerImage(result);
  }, []);

  const dockerImageBuilt =
    !flags.isDocker ||
    (Boolean(dockerfile.trim()) &&
      Boolean(imageTag.trim()) &&
      builtDockerImage?.dockerfile === dockerfile &&
      builtDockerImage?.imageTag === imageTag.trim());

  const prepareDesc = flags.isRemote
    ? t("executors:prepareScriptDescriptionRemote", { trigger: "{{" })
    : t("executors:prepareScriptDescriptionLocal");

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
    configBundleIds: remoteAuth.configBundleIds,
    setConfigBundleIds: remoteAuth.setConfigBundleIds,
    agentEnvVars: remoteAuth.agentEnvVars,
    handleAgentEnvVarChange: remoteAuth.handleAgentEnvVarChange,
    localGitIdentity: gitIdentity.localGitIdentity,
    gitIdentityMode: gitIdentity.gitIdentityMode,
    setGitIdentityMode: gitIdentity.setGitIdentityMode,
    dockerfile,
    setDockerfile,
    imageTag,
    setImageTag,
    recordDockerBuildSuccess,
    dockerImageBuilt,
    gitUserName: gitIdentity.gitUserName,
    setGitUserName: gitIdentity.setGitUserName,
    gitUserEmail: gitIdentity.gitUserEmail,
    setGitUserEmail: gitIdentity.setGitUserEmail,
    isRemote: flags.isRemote,
    isDocker: flags.isDocker,
    isSprites: flags.isSprites,
    mcpPolicyErrorKey,
    buildEnvVars,
    prepareDesc,
  };
}

function useCreateProfileSave(executorId: string) {
  const { t } = useTranslation();
  const router = useRouter();
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = useCallback(
    async (payload: ReturnType<typeof buildCreateProfilePayload>) => {
      setSaving(true);
      setError(null);
      try {
        const profile = await createExecutorProfile(executorId, payload);
        setExecutors(
          executors.map((e: Executor) =>
            e.id === executorId ? { ...e, profiles: [...(e.profiles ?? []), profile] } : e,
          ),
        );
        runWithNavigationBlockerBypassed(() => router.push(`/settings/executors/${profile.id}`));
      } catch (err) {
        setError(err instanceof Error ? err.message : t("executors:failedToCreateProfile"));
        throw err;
      } finally {
        setSaving(false);
      }
    },
    [executorId, executors, setExecutors, router, t],
  );

  return { saving, error, handleSave };
}

function CreateProfileSections({
  executorType,
  form,
  secrets,
}: {
  executorType: ExecutorType;
  form: ReturnType<typeof useCreateProfileFormState>;
  secrets: ReturnType<typeof useSecrets>["items"];
}) {
  const { t } = useTranslation();
  return (
    <>
      <ProfileDetailsCard name={form.name} baselineName="" onNameChange={form.setName} />
      {form.isSprites && (
        <SpritesApiKeyCard
          secretId={form.spritesSecretId}
          baselineSecretId={null}
          onSecretIdChange={form.setSpritesSecretId}
          secrets={secrets}
        />
      )}
      {form.isDocker && (
        <DockerfileBuildCard
          dockerfile={form.dockerfile}
          baselineDockerfile=""
          onDockerfileChange={form.setDockerfile}
          imageTag={form.imageTag}
          baselineImageTag=""
          onImageTagChange={form.setImageTag}
          onBuildSuccess={form.recordDockerBuildSuccess}
        />
      )}
      <CreateRemoteCredentialsSection executorType={executorType} form={form} secrets={secrets} />
      {form.isSprites && (
        <NetworkPoliciesCard
          rules={form.networkPolicyRules}
          baselineRules={[]}
          onRulesChange={form.setNetworkPolicyRules}
        />
      )}
      <EnvVarsCard
        rows={form.envVarRows}
        baselineRows={[]}
        secrets={secrets}
        onAdd={form.addEnvVar}
        onUpdate={form.updateEnvVar}
        onRemove={form.removeEnvVar}
      />
      <ScriptCard
        title={t("executors:prepareScript")}
        description={form.prepareDesc}
        value={form.prepareScript}
        baselineValue=""
        onChange={form.setPrepareScript}
        height="300px"
        placeholders={form.placeholders}
        executorType={executorType}
      />
      {form.isRemote && (
        <ScriptCard
          title={t("executors:cleanupScript")}
          description={t("executors:runsAfterTheAgentSessionEnds")}
          value={form.cleanupScript}
          baselineValue=""
          onChange={form.setCleanupScript}
          height="200px"
          placeholders={form.placeholders}
          executorType={executorType}
        />
      )}
      <McpPolicyCard
        mcpPolicy={form.mcpPolicy}
        baselinePolicy=""
        mcpPolicyErrorKey={form.mcpPolicyErrorKey}
        onPolicyChange={form.setMcpPolicy}
      />
    </>
  );
}

function CreateRemoteCredentialsSection({
  executorType,
  form,
  secrets,
}: {
  executorType: ExecutorType;
  form: ReturnType<typeof useCreateProfileFormState>;
  secrets: ReturnType<typeof useSecrets>["items"];
}) {
  if (!form.isRemote) return null;
  return (
    <RemoteCredentialsCard
      isRemote
      selectedIds={form.remoteCredentials}
      baselineSelectedIds={[]}
      onChange={form.setRemoteCredentials}
      configBundleIds={form.configBundleIds}
      onConfigBundleChange={form.setConfigBundleIds}
      isSSH={executorType === "ssh"}
      agentEnvVars={form.agentEnvVars}
      baselineAgentEnvVars={{}}
      onAgentEnvVarChange={form.handleAgentEnvVarChange}
      secrets={secrets}
      gitIdentityMode={form.gitIdentityMode}
      baselineGitIdentityMode="override"
      onGitIdentityModeChange={form.setGitIdentityMode}
      gitUserName={form.gitUserName}
      gitUserEmail={form.gitUserEmail}
      baselineGitUserName=""
      baselineGitUserEmail=""
      onGitUserNameChange={form.setGitUserName}
      onGitUserEmailChange={form.setGitUserEmail}
      localGitIdentity={form.localGitIdentity}
    />
  );
}

// Returns a catalog key (or null when the form is submittable) so the reason
// resolves at render rather than at module load.
function getCreateDisabledReasonKey(
  form: ReturnType<typeof useCreateProfileFormState>,
  spritesTokenMissing: boolean,
  saving: boolean,
) {
  if (saving) return "executors:creatingProfile";
  if (!form.name.trim()) return "executors:enterAProfileName";
  if (form.mcpPolicyErrorKey) return form.mcpPolicyErrorKey;
  if (spritesTokenMissing) return "executors:addASpritesApiKeyBeforeCreating";
  if (form.isDocker) {
    if (!form.imageTag.trim()) return "executors:enterAnImageTagBeforeCreating";
    if (!form.dockerfile.trim()) return "executors:addDockerfileContentBeforeCreating";
    if (!form.dockerImageBuilt) return "executors:buildThisDockerImageBeforeCreating";
  }
  return null;
}

function buildCreateProfilePayload(form: ReturnType<typeof useCreateProfileFormState>) {
  return {
    name: form.name.trim(),
    mcp_policy: form.mcpPolicy || undefined,
    config: buildProfileConfig({
      isRemote: form.isRemote,
      isSprites: form.isSprites,
      isDocker: form.isDocker,
      networkPolicyRules: form.networkPolicyRules,
      remoteCredentials: form.remoteCredentials,
      configBundleIds: form.configBundleIds,
      agentEnvVars: form.agentEnvVars,
      gitIdentityMode: form.gitIdentityMode,
      localGitIdentity: form.localGitIdentity,
      gitUserName: form.gitUserName,
      gitUserEmail: form.gitUserEmail,
      dockerfile: form.dockerfile,
      imageTag: form.imageTag,
    }),
    prepare_script: form.prepareScript,
    cleanup_script: form.cleanupScript,
    env_vars: form.buildEnvVars(),
  };
}

function CreateProfileForm({
  executorType,
  typeInfo,
}: {
  executorType: ExecutorType;
  typeInfo: ExecutorTypeInfo;
}) {
  const { t } = useTranslation();
  const { items: secrets } = useSecrets();
  const form = useCreateProfileFormState(executorType);
  const { saving, error, handleSave } = useCreateProfileSave(typeInfo.executorId);
  const spritesTokenMissing = form.isSprites && !form.spritesSecretId;
  const disabledReasonKey = getCreateDisabledReasonKey(form, spritesTokenMissing, saving);
  const savePayload = buildCreateProfilePayload(form);
  const saveRevision = serializeSettingsRevision(savePayload);
  useSettingsSaveContributor({
    id: `executor-profile:new:${typeInfo.executorId}`,
    revision: saveRevision,
    isDirty: true,
    canSave: !disabledReasonKey,
    invalidReason: disabledReasonKey ? t(disabledReasonKey) : undefined,
    save: () => handleSave(savePayload),
    discard: () => undefined,
  });

  return (
    <div className="space-y-8">
      <CreateProfileHeader type={executorType} typeInfo={typeInfo} />
      <fieldset disabled={saving} className="space-y-8">
        <CreateProfileSections executorType={executorType} form={form} secrets={secrets} />
      </fieldset>
      {spritesTokenMissing && (
        <p className="text-sm text-destructive">{t("executors:spritesApiKeyIsRequired")}</p>
      )}
      {error && <p className="text-sm text-destructive">{error}</p>}
    </div>
  );
}
