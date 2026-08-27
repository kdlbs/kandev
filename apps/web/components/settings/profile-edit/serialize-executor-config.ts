import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";

export type ExecutorProfileConfigForm = {
  isSprites: boolean;
  networkPolicyRules: NetworkPolicyRule[];
  isRemote: boolean;
  remoteCredentials: string[];
  configBundleIds: string[];
  agentEnvVars: Record<string, string | null>;
  gitIdentityMode: "local" | "override";
  localGitIdentity: { userName: string; userEmail: string };
  gitUserName: string;
  gitUserEmail: string;
  isDocker: boolean;
  dockerfile: string;
  imageTag: string;
  isSSH: boolean;
  sshShell: string;
  sshReclaimTaskDir: boolean;
};

export function buildSaveConfig(
  form: ExecutorProfileConfigForm,
  baseConfig: Record<string, string> = {},
): Record<string, string> {
  const config = { ...baseConfig };
  setJsonConfig(config, "sprites_network_policy_rules", form.isSprites, form.networkPolicyRules);
  setJsonConfig(config, "remote_credentials", form.isRemote, form.remoteCredentials);
  setJsonConfig(config, "agent_config_bundles", form.isRemote, form.configBundleIds);
  const envVars = Object.fromEntries(
    Object.entries(form.agentEnvVars).filter(([, v]) => v != null),
  );
  setJsonConfig(config, "remote_auth_secrets", form.isRemote, envVars);

  const gitName =
    form.gitIdentityMode === "local" ? form.localGitIdentity.userName : form.gitUserName;
  const gitEmail =
    form.gitIdentityMode === "local" ? form.localGitIdentity.userEmail : form.gitUserEmail;
  setTextConfig(config, "git_user_name", form.isRemote ? gitName.trim() : "");
  setTextConfig(config, "git_user_email", form.isRemote ? gitEmail.trim() : "");
  setTextConfig(config, "dockerfile", form.isDocker ? form.dockerfile : "");
  setTextConfig(config, "image_tag", form.isDocker ? form.imageTag.trim() : "");
  setTextConfig(config, "ssh_shell", form.isSSH ? form.sshShell.trim() : "");
  setBoolConfig(config, "ssh_reclaim_task_dir", form.isSSH, form.sshReclaimTaskDir);
  return config;
}

function setJsonConfig(
  config: Record<string, string>,
  key: string,
  enabled: boolean,
  value: unknown,
): void {
  const hasValue = Array.isArray(value)
    ? value.length > 0
    : value !== null && typeof value === "object" && Object.keys(value).length > 0;
  if (enabled && hasValue) config[key] = JSON.stringify(value);
  else delete config[key];
}

// setBoolConfig writes an explicit "true"/"false" rather than deleting the key
// when off, so a profile records that reclamation was considered and declined.
// The backend treats anything other than "true" as disabled, so both the
// absent and the "false" case keep the pre-existing keep-forever behavior.
function setBoolConfig(
  config: Record<string, string>,
  key: string,
  enabled: boolean,
  value: boolean,
): void {
  if (!enabled) {
    delete config[key];
    return;
  }
  config[key] = value ? "true" : "false";
}

function setTextConfig(config: Record<string, string>, key: string, value: string): void {
  if (value && value.trim()) config[key] = value;
  else delete config[key];
}
