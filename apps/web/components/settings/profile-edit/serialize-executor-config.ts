import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import type { ExecutorType } from "@/lib/types/http";
import type { AdditionalNetworkRow } from "@/components/settings/profile-edit/use-docker-networks-form-state";

export function getExecutorProfileRuntimeFlags(executorType: ExecutorType) {
  const isRemote =
    executorType === "local_docker" ||
    executorType === "remote_docker" ||
    executorType === "sprites" ||
    executorType === "ssh" ||
    executorType === "k8s";
  return {
    isRemote,
    isDocker: executorType === "local_docker" || executorType === "remote_docker",
    isSprites: executorType === "sprites",
    isKubernetes: executorType === "k8s",
  };
}

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
  isLocalDocker: boolean;
  dockerfile: string;
  imageTag: string;
  allowUserNamespaces: boolean;
  isSSH: boolean;
  sshShell: string;
  sshReclaimTaskDir: boolean;
  primaryNetwork: string;
  primaryGwPriority: string;
  additionalNetworks: AdditionalNetworkRow[];
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
  setTextConfig(
    config,
    "allow_user_namespaces",
    form.isLocalDocker && form.allowUserNamespaces ? "true" : "",
  );
  setTextConfig(config, "ssh_shell", form.isSSH ? form.sshShell.trim() : "");
  setBoolConfig(config, "ssh_reclaim_task_dir", form.isSSH, form.sshReclaimTaskDir);
  applyDockerNetworkConfig(config, form);
  return config;
}

// applyDockerNetworkConfig writes the three network keys, or clears them.
//
// A gateway priority without a primary network is refused by the backend at
// launch, so the editor drops it rather than saving a profile whose only
// symptom is a failed launch.
function applyDockerNetworkConfig(
  config: Record<string, string>,
  form: ExecutorProfileConfigForm,
): void {
  const primary = form.isDocker ? form.primaryNetwork.trim() : "";
  setTextConfig(config, "docker_network", primary);
  setTextConfig(
    config,
    "docker_network_gw_priority",
    primary ? form.primaryGwPriority.trim() : "",
  );

  const additional = form.isDocker
    ? form.additionalNetworks
        .map((row) => ({ name: row.name.trim(), gwPriority: row.gwPriority.trim() }))
        .filter((row) => row.name !== "")
        .map((row) =>
          row.gwPriority === ""
            ? { name: row.name }
            : { name: row.name, gw_priority: Number(row.gwPriority) },
        )
    : [];
  setJsonConfig(config, "docker_additional_networks", form.isDocker, additional);
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
