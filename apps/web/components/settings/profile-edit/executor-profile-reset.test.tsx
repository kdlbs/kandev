import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useProfileFormState } from "@/app/settings/executors/[profileId]/page";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { envVarsToRows } from "./env-vars-card";
import {
  deriveSpritesSecretId,
  getGitIdentityBaseline,
  parseAgentConfigBundles,
  parseNetworkPolicyRules,
  parseRemoteAuthSecrets,
  parseRemoteCredentials,
} from "./executor-profile-baselines";
import { parseKubernetesProfileConfig } from "../kubernetes-config";

vi.mock("@/lib/api/domains/settings-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/settings-api")>();
  return {
    ...actual,
    fetchLocalGitIdentity: vi.fn(async () => ({
      user_name: "Detected User",
      user_email: "detected@example.com",
      detected: true,
    })),
    listScriptPlaceholders: vi.fn(async () => ({ placeholders: [] })),
  };
});

const PERSISTED_AT = "2026-08-24T10:00:00Z";

const profile: ExecutorProfile = {
  id: "profile-1",
  executor_id: "executor-1",
  name: "Persisted profile",
  mcp_policy: '{"allow_http":true}',
  prepare_script: "echo prepare",
  cleanup_script: "echo cleanup",
  env_vars: [
    { key: "REGULAR_ENV", value: "saved value" },
    { key: "SPRITES_API_TOKEN", secret_id: "sprites-secret" },
  ],
  config: {
    dockerfile: "FROM persisted",
    image_tag: "persisted:latest",
    ssh_shell: "/bin/zsh",
    ssh_reclaim_task_dir: "true",
    platform: "linux/arm64",
    main_container: "persisted-agent",
    pod_template_yaml: "persisted pod template",
    "workspace.mode": "managed_pvc",
    "workspace.size": "20Gi",
    "workspace.storage_class": "fast",
    "workspace.access_modes": '["ReadWriteMany"]',
    sprites_network_policy_rules: '[{"domain":"saved.example","action":"allow"}]',
    remote_credentials: '["credential-1"]',
    agent_config_bundles: '["bundle-1"]',
    remote_auth_secrets: '{"codex":"agent-secret"}',
    git_user_name: "Persisted User",
    git_user_email: "persisted@example.com",
  },
  created_at: PERSISTED_AT,
  updated_at: PERSISTED_AT,
};

const executor: Executor = {
  id: "executor-1",
  name: "Sprites executor",
  type: "sprites",
  status: "active",
  is_system: false,
  profiles: [profile],
  created_at: PERSISTED_AT,
  updated_at: PERSISTED_AT,
};

function editEveryProfileField(form: ReturnType<typeof useProfileFormState>) {
  form.setName("Edited profile");
  form.setMcpPolicy('{"allow_http":false}');
  form.setPrepareScript("edited prepare");
  form.setCleanupScript("edited cleanup");
  form.setDockerfile("FROM edited");
  form.setImageTag("edited:latest");
  form.setSshShell("/bin/bash");
  form.setSshReclaimTaskDir(false);
  form.setKubernetesProfile({
    ...form.kubernetesProfile,
    mainContainer: "edited-agent",
    podTemplateYaml: "edited pod template",
    workspaceSize: "1Gi",
  });
  form.updateEnvVar(0, "value", "edited value");
  form.addEnvVar({ key: "NEW_ENV", mode: "value", value: "new", secretId: "" });
  form.setSpritesSecretId("edited-sprites-secret");
  form.setNetworkPolicyRules([{ domain: "edited.example", action: "deny" }]);
  form.setRemoteCredentials(["credential-2"]);
  form.setConfigBundleIds(["bundle-2"]);
  form.handleAgentEnvVarChange("codex", "edited-agent-secret");
  form.setGitIdentityMode("local");
  form.setGitUserName("Edited User");
  form.setGitUserEmail("edited@example.com");
}

describe("executor profile reset", () => {
  it("restores every editable field from the persisted profile baseline", async () => {
    const view = renderHook(() => useProfileFormState(executor, profile));
    await waitFor(() => expect(view.result.current.gitIdentityLoaded).toBe(true));

    act(() => editEveryProfileField(view.result.current));

    const reset = Reflect.get(view.result.current, "reset");
    expect(typeof reset).toBe("function");
    act(() => reset());

    const form = view.result.current;
    const gitBaseline = getGitIdentityBaseline(profile, form.localGitIdentity);
    expect({
      name: form.name,
      mcpPolicy: form.mcpPolicy,
      prepareScript: form.prepareScript,
      cleanupScript: form.cleanupScript,
      dockerfile: form.dockerfile,
      imageTag: form.imageTag,
      sshShell: form.sshShell,
      sshReclaimTaskDir: form.sshReclaimTaskDir,
      kubernetesProfile: form.kubernetesProfile,
      envVarRows: form.envVarRows,
      spritesSecretId: form.spritesSecretId,
      networkPolicyRules: form.networkPolicyRules,
      remoteCredentials: form.remoteCredentials,
      configBundleIds: form.configBundleIds,
      agentEnvVars: form.agentEnvVars,
      gitIdentityMode: form.gitIdentityMode,
      gitUserName: form.gitUserName,
      gitUserEmail: form.gitUserEmail,
    }).toEqual({
      name: profile.name,
      mcpPolicy: profile.mcp_policy,
      prepareScript: profile.prepare_script,
      cleanupScript: profile.cleanup_script,
      dockerfile: profile.config?.dockerfile,
      imageTag: profile.config?.image_tag,
      sshShell: profile.config?.ssh_shell,
      sshReclaimTaskDir: true,
      kubernetesProfile: parseKubernetesProfileConfig(profile.config),
      envVarRows: envVarsToRows(profile.env_vars),
      spritesSecretId: deriveSpritesSecretId(profile.env_vars),
      networkPolicyRules: parseNetworkPolicyRules(profile.config),
      remoteCredentials: parseRemoteCredentials(profile.config),
      configBundleIds: parseAgentConfigBundles(profile.config),
      agentEnvVars: parseRemoteAuthSecrets(profile.config),
      gitIdentityMode: gitBaseline.mode,
      gitUserName: gitBaseline.userName,
      gitUserEmail: gitBaseline.userEmail,
    });
  });

  it("uses the current persisted profile after the baseline is refreshed", () => {
    const updatedProfile = {
      ...profile,
      name: "Server-updated profile",
      prepare_script: "server-updated prepare",
      config: { ...profile.config, pod_template_yaml: "server-updated pod template" },
      env_vars: [{ key: "SERVER_ENV", value: "server value" }],
      updated_at: "2026-08-24T11:00:00Z",
    };
    const view = renderHook(({ currentProfile }) => useProfileFormState(executor, currentProfile), {
      initialProps: { currentProfile: profile },
    });

    view.rerender({ currentProfile: updatedProfile });
    act(() => {
      view.result.current.setName("Local edit");
      view.result.current.setPrepareScript("local prepare");
      view.result.current.setKubernetesProfile({
        ...view.result.current.kubernetesProfile,
        podTemplateYaml: "local pod template",
      });
      view.result.current.addEnvVar({
        key: "LOCAL_ENV",
        mode: "value",
        value: "local value",
        secretId: "",
      });
    });
    act(() => view.result.current.reset());

    expect({
      name: view.result.current.name,
      prepareScript: view.result.current.prepareScript,
      podTemplateYaml: view.result.current.kubernetesProfile.podTemplateYaml,
      envVarRows: view.result.current.envVarRows,
    }).toEqual({
      name: updatedProfile.name,
      prepareScript: updatedProfile.prepare_script,
      podTemplateYaml: updatedProfile.config.pod_template_yaml,
      envVarRows: envVarsToRows(updatedProfile.env_vars),
    });
  });
});
