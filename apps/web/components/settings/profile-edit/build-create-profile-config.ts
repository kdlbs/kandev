import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import type {
  GitIdentityMode,
  GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import type { AdditionalNetworkRow } from "@/components/settings/profile-edit/use-docker-networks-form-state";
import { applyDockerNetworks } from "@/components/settings/profile-edit/build-docker-network-config";

export type BuildProfileConfigInput = {
  isRemote: boolean;
  isSprites: boolean;
  isDocker: boolean;
  isLocalDocker: boolean;
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
  allowUserNamespaces: boolean;
  primaryNetwork: string;
  primaryGwPriority: string;
  additionalNetworks: AdditionalNetworkRow[];
};

export function buildProfileConfig(
  input: BuildProfileConfigInput,
): Record<string, string> | undefined {
  const {
    isRemote,
    isSprites,
    networkPolicyRules,
    remoteCredentials,
    configBundleIds,
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
  applyDockerCreateConfig(config, input);
  return Object.keys(config).length > 0 ? config : undefined;
}

function applyDockerCreateConfig(
  config: Record<string, string>,
  input: BuildProfileConfigInput,
): void {
  if (input.isDocker && input.dockerfile.trim()) {
    config.dockerfile = input.dockerfile;
  }
  if (input.isDocker && input.imageTag.trim()) {
    config.image_tag = input.imageTag.trim();
  }
  if (input.isLocalDocker && input.allowUserNamespaces) {
    config.allow_user_namespaces = "true";
  }
  applyDockerNetworks(config, input);
}
