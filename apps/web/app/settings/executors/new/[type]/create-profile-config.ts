import type {
  GitIdentityMode,
  GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import { buildCursorCloudProfileConfig } from "@/components/settings/profile-edit/cursor-cloud-profile-config";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";

type BuildProfileConfigInput = {
  isCursorCloud: boolean;
  cursorCloudSecretId: string | null;
  cursorCloudCallbackUrl: string;
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
};

export function buildProfileConfig(
  input: BuildProfileConfigInput,
): Record<string, string> | undefined {
  const { isSprites, networkPolicyRules } = input;
  const config: Record<string, string> = {};
  if (isSprites && networkPolicyRules.length > 0) {
    config.sprites_network_policy_rules = JSON.stringify(networkPolicyRules);
  }
  applyRemoteConfig(config, input);
  applyRemoteGitIdentity(config, input);
  applyDockerCreateConfig(config, input);
  const finalConfig = input.isCursorCloud
    ? buildCursorCloudProfileConfig(config, input.cursorCloudSecretId, input.cursorCloudCallbackUrl)
    : config;
  return Object.keys(finalConfig).length > 0 ? finalConfig : undefined;
}

function applyRemoteConfig(config: Record<string, string>, input: BuildProfileConfigInput): void {
  const { isRemote, remoteCredentials, configBundleIds, agentEnvVars } = input;
  if (isRemote && remoteCredentials.length > 0) {
    config.remote_credentials = JSON.stringify(remoteCredentials);
  }
  if (isRemote && configBundleIds.length > 0) {
    config.agent_config_bundles = JSON.stringify(configBundleIds);
  }
  const nonNullEnvVars = Object.fromEntries(
    Object.entries(agentEnvVars).filter(([, value]) => value != null),
  );
  if (isRemote && Object.keys(nonNullEnvVars).length > 0) {
    config.remote_auth_secrets = JSON.stringify(nonNullEnvVars);
  }
}

function applyRemoteGitIdentity(
  config: Record<string, string>,
  input: BuildProfileConfigInput,
): void {
  if (!input.isRemote) return;
  const effectiveName =
    input.gitIdentityMode === "local"
      ? input.localGitIdentity.userName.trim()
      : input.gitUserName.trim();
  const effectiveEmail =
    input.gitIdentityMode === "local"
      ? input.localGitIdentity.userEmail.trim()
      : input.gitUserEmail.trim();
  if (effectiveName) config.git_user_name = effectiveName;
  if (effectiveEmail) config.git_user_email = effectiveEmail;
}

function applyDockerCreateConfig(
  config: Record<string, string>,
  input: BuildProfileConfigInput,
): void {
  if (input.isDocker && input.dockerfile.trim()) config.dockerfile = input.dockerfile;
  if (input.isDocker && input.imageTag.trim()) config.image_tag = input.imageTag.trim();
  if (input.isLocalDocker && input.allowUserNamespaces) config.allow_user_namespaces = "true";
}
