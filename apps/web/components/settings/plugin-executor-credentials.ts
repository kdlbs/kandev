"use client";

import { useCallback, useEffect, useState } from "react";
import type { ExecutorProfile } from "@/lib/types/http";
import {
  getGitIdentityBaseline,
  parseAgentConfigBundles,
  parseRemoteAuthSecrets,
  parseRemoteCredentials,
} from "@/components/settings/profile-edit/executor-profile-baselines";
import type {
  GitIdentityMode,
  GitIdentityState,
} from "@/components/settings/profile-edit/remote-credentials-card";
import { buildSaveConfig } from "@/components/settings/profile-edit/serialize-executor-config";
import { useGitIdentityState } from "@/components/settings/profile-edit/use-git-identity-state";

// Kandev-owned plugin executor profile keys. An empty value asks the backend to
// remove the key, so every key is always sent.
const PLUGIN_CREDENTIAL_KEYS = [
  "remote_credentials",
  "agent_config_bundles",
  "remote_auth_secrets",
  "git_user_name",
  "git_user_email",
] as const;

export type PluginCredentialForm = {
  remoteCredentials: string[];
  configBundleIds: string[];
  agentEnvVars: Record<string, string | null>;
  gitIdentityMode: GitIdentityMode;
  localGitIdentity: GitIdentityState;
  gitUserName: string;
  gitUserEmail: string;
};

export function pluginCredentialConfig(form: PluginCredentialForm): Record<string, string> {
  const config = buildSaveConfig({
    ...form,
    isRemote: true,
    isSprites: false,
    networkPolicyRules: [],
    isDocker: false,
    isLocalDocker: false,
    dockerfile: "",
    imageTag: "",
    allowUserNamespaces: false,
    isSSH: false,
    sshShell: "",
    sshReclaimTaskDir: false,
    primaryNetwork: "",
    primaryGwPriority: "",
    additionalNetworks: [],
  });
  return Object.fromEntries(PLUGIN_CREDENTIAL_KEYS.map((key) => [key, config[key] ?? ""]));
}

function baselineForm(profile: ExecutorProfile, local: GitIdentityState): PluginCredentialForm {
  const git = getGitIdentityBaseline(profile, local);
  return {
    remoteCredentials: parseRemoteCredentials(profile.config),
    configBundleIds: parseAgentConfigBundles(profile.config),
    agentEnvVars: parseRemoteAuthSecrets(profile.config),
    gitIdentityMode: git.mode,
    localGitIdentity: local,
    gitUserName: git.userName,
    gitUserEmail: git.userEmail,
  };
}

// usePluginExecutorCredentials holds the agent credential selection of a plugin
// executor profile, edited with the same card as the other remote executors.
export function usePluginExecutorCredentials(profile: ExecutorProfile) {
  const git = useGitIdentityState(true, profile);
  const [remoteCredentials, setRemoteCredentials] = useState(() =>
    parseRemoteCredentials(profile.config),
  );
  const [configBundleIds, setConfigBundleIds] = useState(() =>
    parseAgentConfigBundles(profile.config),
  );
  const [agentEnvVars, setAgentEnvVars] = useState(() => parseRemoteAuthSecrets(profile.config));

  const resetSelections = useCallback(() => {
    setRemoteCredentials(parseRemoteCredentials(profile.config));
    setConfigBundleIds(parseAgentConfigBundles(profile.config));
    setAgentEnvVars(parseRemoteAuthSecrets(profile.config));
  }, [profile.config]);
  // Reload the draft only when the stored profile changes, so a late local Git
  // identity response cannot overwrite unsaved selections.
  useEffect(resetSelections, [resetSelections]);
  const { reset: resetGit } = git;
  const reset = useCallback(() => {
    resetSelections();
    resetGit();
  }, [resetSelections, resetGit]);

  const form: PluginCredentialForm = {
    remoteCredentials,
    configBundleIds,
    agentEnvVars,
    gitIdentityMode: git.gitIdentityMode,
    localGitIdentity: git.localGitIdentity,
    gitUserName: git.gitUserName,
    gitUserEmail: git.gitUserEmail,
  };
  const baseline = baselineForm(profile, git.localGitIdentity);
  const config = pluginCredentialConfig(form);
  const dirty =
    git.loaded && JSON.stringify(config) !== JSON.stringify(pluginCredentialConfig(baseline));

  return {
    form,
    baseline,
    config,
    dirty,
    reset,
    setRemoteCredentials,
    setConfigBundleIds,
    onAgentEnvVarChange: useCallback(
      (methodId: string, secretId: string | null) =>
        setAgentEnvVars((current) => ({ ...current, [methodId]: secretId })),
      [],
    ),
    setGitIdentityMode: git.setGitIdentityMode,
    setGitUserName: git.setGitUserName,
    setGitUserEmail: git.setGitUserEmail,
  };
}
