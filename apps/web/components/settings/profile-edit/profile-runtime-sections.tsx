"use client";

import { useState } from "react";
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

export function DockerSections({ profile }: { profile: ExecutorProfile }) {
  const [dockerfile, setDockerfile] = useState(profile.config?.dockerfile ?? "");
  const [imageTag, setImageTag] = useState(profile.config?.image_tag ?? "");

  return (
    <>
      <DockerfileBuildCard
        dockerfile={dockerfile}
        onDockerfileChange={setDockerfile}
        imageTag={imageTag}
        onImageTagChange={setImageTag}
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
  onNetworkRulesChange: (rules: NetworkPolicyRule[]) => void;
  remoteCredentials: string[];
  onRemoteCredentialsChange: (ids: string[]) => void;
  agentEnvVars: Record<string, string | null>;
  onAgentEnvVarChange: (agentId: string, secretId: string | null) => void;
  gitIdentityMode: GitIdentityMode;
  onGitIdentityModeChange: (mode: GitIdentityMode) => void;
  gitUserName: string;
  gitUserEmail: string;
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
  onNetworkRulesChange,
  remoteCredentials,
  onRemoteCredentialsChange,
  agentEnvVars,
  onAgentEnvVarChange,
  gitIdentityMode,
  onGitIdentityModeChange,
  gitUserName,
  gitUserEmail,
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
        onChange={onRemoteCredentialsChange}
        agentEnvVars={agentEnvVars}
        onAgentEnvVarChange={onAgentEnvVarChange}
        secrets={secrets}
        gitIdentityMode={gitIdentityMode}
        onGitIdentityModeChange={onGitIdentityModeChange}
        gitUserName={gitUserName}
        gitUserEmail={gitUserEmail}
        onGitUserNameChange={onGitUserNameChange}
        onGitUserEmailChange={onGitUserEmailChange}
        localGitIdentity={localGitIdentity}
      />
      {isSprites && (
        <NetworkPoliciesCard rules={networkRules} onRulesChange={onNetworkRulesChange} />
      )}
    </>
  );
}
