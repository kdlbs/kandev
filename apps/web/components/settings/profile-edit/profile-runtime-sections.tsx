"use client";

import type { ExecutorProfile } from "@/lib/types/http";
import type { SecretListItem } from "@/lib/types/http-secrets";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import {
  RemoteCredentialsCard,
  type GitIdentityMode,
  type GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import {
  DockerfileBuildCard,
  DockerContainersCard,
} from "@/components/settings/profile-edit/docker-sections";
import { NetworkPoliciesCard } from "@/components/settings/profile-edit/sprites-sections";
import { SpritesInstancesCard } from "@/components/settings/sprites-settings";

type DockerSectionsProps = {
  profile: ExecutorProfile;
  dockerfile: string;
  onDockerfileChange: (v: string) => void;
  imageTag: string;
  onImageTagChange: (v: string) => void;
};

export function DockerSections({
  profile,
  dockerfile,
  onDockerfileChange,
  imageTag,
  onImageTagChange,
}: DockerSectionsProps) {
  return (
    <>
      <DockerfileBuildCard
        dockerfile={dockerfile}
        baselineDockerfile={profile.config?.dockerfile ?? ""}
        onDockerfileChange={onDockerfileChange}
        imageTag={imageTag}
        baselineImageTag={profile.config?.image_tag ?? ""}
        onImageTagChange={onImageTagChange}
      />
      <DockerContainersCard profileId={profile.id} />
    </>
  );
}

type SpritesSectionsProps = {
  isRemote: boolean;
  isSprites: boolean;
  secretId: string | null;
  networkRules: NetworkPolicyRule[];
  baselineNetworkRules?: NetworkPolicyRule[];
  onNetworkRulesChange: (rules: NetworkPolicyRule[]) => void;
  remoteCredentials: string[];
  baselineRemoteCredentials?: string[];
  onRemoteCredentialsChange: (ids: string[]) => void;
  agentEnvVars: Record<string, string | null>;
  baselineAgentEnvVars?: Record<string, string | null>;
  onAgentEnvVarChange: (agentId: string, secretId: string | null) => void;
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
  secrets: SecretListItem[];
};

export function SpritesSections({
  isRemote,
  isSprites,
  secretId,
  networkRules,
  baselineNetworkRules,
  onNetworkRulesChange,
  remoteCredentials,
  baselineRemoteCredentials,
  onRemoteCredentialsChange,
  agentEnvVars,
  baselineAgentEnvVars,
  onAgentEnvVarChange,
  gitIdentityMode,
  baselineGitIdentityMode,
  onGitIdentityModeChange,
  gitUserName,
  gitUserEmail,
  baselineGitUserName,
  baselineGitUserEmail,
  onGitUserNameChange,
  onGitUserEmailChange,
  localGitIdentity,
  secrets,
}: SpritesSectionsProps) {
  return (
    <>
      {isSprites && secretId && <SpritesInstancesCard secretId={secretId} />}
      <RemoteCredentialsCard
        isRemote={isRemote}
        selectedIds={remoteCredentials}
        baselineSelectedIds={baselineRemoteCredentials}
        onChange={onRemoteCredentialsChange}
        agentEnvVars={agentEnvVars}
        baselineAgentEnvVars={baselineAgentEnvVars}
        onAgentEnvVarChange={onAgentEnvVarChange}
        secrets={secrets}
        gitIdentityMode={gitIdentityMode}
        baselineGitIdentityMode={baselineGitIdentityMode}
        onGitIdentityModeChange={onGitIdentityModeChange}
        gitUserName={gitUserName}
        gitUserEmail={gitUserEmail}
        baselineGitUserName={baselineGitUserName}
        baselineGitUserEmail={baselineGitUserEmail}
        onGitUserNameChange={onGitUserNameChange}
        onGitUserEmailChange={onGitUserEmailChange}
        localGitIdentity={localGitIdentity}
      />
      {isSprites && (
        <NetworkPoliciesCard
          rules={networkRules}
          baselineRules={baselineNetworkRules}
          onRulesChange={onNetworkRulesChange}
        />
      )}
    </>
  );
}
