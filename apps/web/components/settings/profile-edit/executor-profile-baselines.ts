import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import type { ExecutorProfile, ProfileEnvVar } from "@/lib/types/http";
import type { GitIdentityMode, GitIdentityState } from "./remote-credentials-card";

const SPRITES_TOKEN_KEY = "SPRITES_API_TOKEN";

export function deriveSpritesSecretId(envVars?: ProfileEnvVar[]): string | null {
  const row = envVars?.find((envVar) => envVar.key === SPRITES_TOKEN_KEY && envVar.secret_id);
  return row?.secret_id ?? null;
}

export function parseNetworkPolicyRules(config?: Record<string, string>): NetworkPolicyRule[] {
  return parseJsonSetting(config?.sprites_network_policy_rules, []);
}

export function parseRemoteCredentials(config?: Record<string, string>): string[] {
  return parseJsonSetting(config?.remote_credentials, []);
}

export function parseRemoteAuthSecrets(
  config?: Record<string, string>,
): Record<string, string | null> {
  return parseJsonSetting(config?.remote_auth_secrets, {});
}

export function getGitIdentityBaseline(
  profile: ExecutorProfile,
  localGitIdentity: GitIdentityState,
): {
  mode: GitIdentityMode;
  userName: string;
  userEmail: string;
} {
  const storedName = profile.config?.git_user_name ?? "";
  const storedEmail = profile.config?.git_user_email ?? "";
  if (storedName.trim() || storedEmail.trim()) {
    return { mode: "override", userName: storedName, userEmail: storedEmail };
  }
  if (localGitIdentity.detected) {
    return {
      mode: "local",
      userName: localGitIdentity.userName,
      userEmail: localGitIdentity.userEmail,
    };
  }
  return { mode: "override", userName: "", userEmail: "" };
}

function parseJsonSetting<T>(raw: string | undefined, fallback: T): T {
  if (!raw) return fallback;
  try {
    return JSON.parse(raw) as T;
  } catch {
    return fallback;
  }
}
