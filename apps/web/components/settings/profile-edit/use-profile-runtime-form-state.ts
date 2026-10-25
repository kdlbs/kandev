"use client";

import { useCallback, useState } from "react";
import { isSSHReclaimEnabled } from "@/components/settings/ssh-task-dir-reclamation-card";
import {
  parseKubernetesProfileConfig,
  type KubernetesProfileConfigForm,
} from "@/components/settings/kubernetes-config";
import { getExecutorProfileRuntimeFlags } from "@/components/settings/profile-edit/serialize-executor-config";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

export function useProfileRuntimeFormState(executor: Executor, profile: ExecutorProfile) {
  const flags = getExecutorProfileRuntimeFlags(executor.type);
  const [dockerfile, setDockerfile] = useState(profile.config?.dockerfile ?? "");
  const [imageTag, setImageTag] = useState(profile.config?.image_tag ?? "");
  const [sshShell, setSshShell] = useState(profile.config?.ssh_shell ?? "");
  const [sshReclaimTaskDir, setSshReclaimTaskDir] = useState(() =>
    isSSHReclaimEnabled(profile.config),
  );
  const [kubernetesProfile, setKubernetesProfile] = useState<KubernetesProfileConfigForm>(() =>
    parseKubernetesProfileConfig(profile.config),
  );

  const resetRuntime = useCallback(() => {
    setDockerfile(profile.config?.dockerfile ?? "");
    setImageTag(profile.config?.image_tag ?? "");
    setSshShell(profile.config?.ssh_shell ?? "");
    setSshReclaimTaskDir(isSSHReclaimEnabled(profile.config));
    setKubernetesProfile(parseKubernetesProfileConfig(profile.config));
  }, [profile.config]);

  return {
    dockerfile,
    setDockerfile,
    imageTag,
    setImageTag,
    sshShell,
    setSshShell,
    sshReclaimTaskDir,
    setSshReclaimTaskDir,
    kubernetesProfile,
    setKubernetesProfile,
    isRemote: flags.isRemote,
    isDocker: flags.isDocker,
    isSprites: flags.isSprites,
    isSSH: executor.type === "ssh",
    isKubernetes: flags.isKubernetes,
    resetRuntime,
  };
}
