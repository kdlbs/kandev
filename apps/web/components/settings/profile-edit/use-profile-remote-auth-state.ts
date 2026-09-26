"use client";

import { useCallback, useState } from "react";
import type { NetworkPolicyRule } from "@/lib/api/domains/settings-api";
import {
  parseAgentConfigBundles,
  parseNetworkPolicyRules,
  parseRemoteAuthSecrets,
  parseRemoteCredentials,
} from "@/components/settings/profile-edit/executor-profile-baselines";
import type { ExecutorProfile } from "@/lib/types/http";

export function useProfileRemoteAuthState(profile: ExecutorProfile) {
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
