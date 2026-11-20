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
import { buildSaveConfig } from "./serialize-executor-config";
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
const REFRESHED_AT = "2026-08-24T11:00:00Z";
const SAVED_DOCKER_TMP_SIZE = "4g";
const SAVED_DOCKER_LOG_MAX_SIZE = "200m";
const SAVED_DOCKER_LOG_MAX_FILES = "7";

type DockerStorageFormState = ReturnType<typeof useProfileFormState> & {
  dockerTmpSize: string;
  dockerTmpSizeDirty: boolean;
  setDockerTmpSize: (value: string) => void;
  dockerLogMaxSize: string;
  setDockerLogMaxSize: (value: string) => void;
  dockerLogMaxFiles: string;
  setDockerLogMaxFiles: (value: string) => void;
};

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
    docker_tmp_size: "2g",
    docker_log_max_size: "100m",
    docker_log_max_files: "3",
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
  const dockerStorage = form as DockerStorageFormState;
  dockerStorage.setDockerTmpSize?.(SAVED_DOCKER_TMP_SIZE);
  dockerStorage.setDockerLogMaxSize?.(SAVED_DOCKER_LOG_MAX_SIZE);
  dockerStorage.setDockerLogMaxFiles?.(SAVED_DOCKER_LOG_MAX_FILES);
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
      dockerTmpSize: (form as DockerStorageFormState).dockerTmpSize,
      dockerLogMaxSize: (form as DockerStorageFormState).dockerLogMaxSize,
      dockerLogMaxFiles: (form as DockerStorageFormState).dockerLogMaxFiles,
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
      dockerTmpSize: profile.config?.docker_tmp_size,
      dockerLogMaxSize: profile.config?.docker_log_max_size,
      dockerLogMaxFiles: profile.config?.docker_log_max_files,
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
});

describe("executor profile reset after refresh", () => {
  it("uses the current persisted profile after the baseline is refreshed", () => {
    const updatedProfile = {
      ...profile,
      name: "Server-updated profile",
      prepare_script: "server-updated prepare",
      config: {
        ...profile.config,
        pod_template_yaml: "server-updated pod template",
        docker_tmp_size: SAVED_DOCKER_TMP_SIZE,
        docker_log_max_size: SAVED_DOCKER_LOG_MAX_SIZE,
        docker_log_max_files: SAVED_DOCKER_LOG_MAX_FILES,
      },
      env_vars: [{ key: "SERVER_ENV", value: "server value" }],
      updated_at: REFRESHED_AT,
    };
    const view = renderHook(({ currentProfile }) => useProfileFormState(executor, currentProfile), {
      initialProps: { currentProfile: profile },
    });

    view.rerender({ currentProfile: updatedProfile });
    act(() => {
      view.result.current.setName("Local edit");
      view.result.current.setPrepareScript("local prepare");
      const dockerStorage = view.result.current as DockerStorageFormState;
      dockerStorage.setDockerTmpSize?.("6g");
      dockerStorage.setDockerLogMaxSize?.("300m");
      dockerStorage.setDockerLogMaxFiles?.("9");
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
      dockerTmpSize: (view.result.current as DockerStorageFormState).dockerTmpSize,
      dockerLogMaxSize: (view.result.current as DockerStorageFormState).dockerLogMaxSize,
      dockerLogMaxFiles: (view.result.current as DockerStorageFormState).dockerLogMaxFiles,
      podTemplateYaml: view.result.current.kubernetesProfile.podTemplateYaml,
      envVarRows: view.result.current.envVarRows,
    }).toEqual({
      name: updatedProfile.name,
      prepareScript: updatedProfile.prepare_script,
      dockerTmpSize: updatedProfile.config.docker_tmp_size,
      dockerLogMaxSize: updatedProfile.config.docker_log_max_size,
      dockerLogMaxFiles: updatedProfile.config.docker_log_max_files,
      podTemplateYaml: updatedProfile.config.pod_template_yaml,
      envVarRows: envVarsToRows(updatedProfile.env_vars),
    });
  });
});

describe("executor profile storage synchronization", () => {
  it("keeps refreshed Docker storage settings on an unrelated profile save", async () => {
    const dockerExecutor = { ...executor, type: "local_docker" as const };
    const updatedProfile = {
      ...profile,
      config: {
        ...profile.config,
        docker_tmp_size: SAVED_DOCKER_TMP_SIZE,
        docker_log_max_size: SAVED_DOCKER_LOG_MAX_SIZE,
        docker_log_max_files: SAVED_DOCKER_LOG_MAX_FILES,
      },
      updated_at: REFRESHED_AT,
    };
    const view = renderHook(
      ({ currentProfile }) => useProfileFormState(dockerExecutor, currentProfile),
      { initialProps: { currentProfile: profile } },
    );

    view.rerender({ currentProfile: updatedProfile });
    await waitFor(() => {
      const form = view.result.current as DockerStorageFormState;
      expect([form.dockerTmpSize, form.dockerLogMaxSize, form.dockerLogMaxFiles]).toEqual([
        SAVED_DOCKER_TMP_SIZE,
        SAVED_DOCKER_LOG_MAX_SIZE,
        SAVED_DOCKER_LOG_MAX_FILES,
      ]);
    });

    act(() => view.result.current.setName("Local profile edit"));
    const payload = buildSaveConfig(view.result.current, updatedProfile.config);

    expect(payload).toMatchObject({
      docker_tmp_size: SAVED_DOCKER_TMP_SIZE,
      docker_log_max_size: SAVED_DOCKER_LOG_MAX_SIZE,
      docker_log_max_files: SAVED_DOCKER_LOG_MAX_FILES,
    });
  });

  it("keeps an intentional storage clear while other refreshed fields follow config", async () => {
    const dockerExecutor = { ...executor, type: "local_docker" as const };
    const updatedProfile = {
      ...profile,
      config: {
        ...profile.config,
        docker_tmp_size: SAVED_DOCKER_TMP_SIZE,
        docker_log_max_size: SAVED_DOCKER_LOG_MAX_SIZE,
        docker_log_max_files: SAVED_DOCKER_LOG_MAX_FILES,
      },
      updated_at: REFRESHED_AT,
    };
    const view = renderHook(
      ({ currentProfile }) => useProfileFormState(dockerExecutor, currentProfile),
      { initialProps: { currentProfile: profile } },
    );

    act(() => (view.result.current as DockerStorageFormState).setDockerTmpSize(""));
    view.rerender({ currentProfile: updatedProfile });
    await waitFor(() => {
      const form = view.result.current as DockerStorageFormState;
      expect([form.dockerTmpSize, form.dockerLogMaxSize, form.dockerLogMaxFiles]).toEqual([
        "",
        SAVED_DOCKER_LOG_MAX_SIZE,
        SAVED_DOCKER_LOG_MAX_FILES,
      ]);
    });

    const payload = buildSaveConfig(view.result.current, updatedProfile.config);

    expect(payload.docker_tmp_size).toBeUndefined();
    expect(payload).toMatchObject({
      docker_log_max_size: SAVED_DOCKER_LOG_MAX_SIZE,
      docker_log_max_files: SAVED_DOCKER_LOG_MAX_FILES,
    });
  });
});

describe("executor profile storage save refresh", () => {
  it("normalizes a saved storage draft before an unrelated save", async () => {
    const dockerExecutor = { ...executor, type: "local_docker" as const };
    const view = renderHook(
      ({ currentProfile }) => useProfileFormState(dockerExecutor, currentProfile),
      { initialProps: { currentProfile: profile } },
    );

    act(() => (view.result.current as DockerStorageFormState).setDockerTmpSize(" 4g "));
    const submittedConfig = buildSaveConfig(view.result.current, profile.config);
    expect(submittedConfig.docker_tmp_size).toBe(SAVED_DOCKER_TMP_SIZE);

    const savedProfile = {
      ...profile,
      config: { ...profile.config, ...submittedConfig },
      updated_at: REFRESHED_AT,
    };
    view.rerender({ currentProfile: savedProfile });

    await waitFor(() => {
      const form = view.result.current as DockerStorageFormState;
      expect(form.dockerTmpSize).toBe(SAVED_DOCKER_TMP_SIZE);
      expect(form.dockerTmpSizeDirty).toBe(false);
    });

    act(() => view.result.current.setName("Unrelated profile edit"));
    const unrelatedPayload = buildSaveConfig(view.result.current, savedProfile.config);
    expect(unrelatedPayload.docker_tmp_size).toBe(SAVED_DOCKER_TMP_SIZE);
  });

  it("preserves a newer storage edit while an earlier save refreshes the profile", async () => {
    const dockerExecutor = { ...executor, type: "local_docker" as const };
    const view = renderHook(
      ({ currentProfile }) => useProfileFormState(dockerExecutor, currentProfile),
      { initialProps: { currentProfile: profile } },
    );

    act(() => (view.result.current as DockerStorageFormState).setDockerTmpSize(" 4g "));
    const submittedConfig = buildSaveConfig(view.result.current, profile.config);
    act(() => (view.result.current as DockerStorageFormState).setDockerTmpSize("5g"));

    view.rerender({
      currentProfile: {
        ...profile,
        config: { ...profile.config, ...submittedConfig },
        updated_at: REFRESHED_AT,
      },
    });

    await waitFor(() => {
      const form = view.result.current as DockerStorageFormState;
      expect(form.dockerTmpSize).toBe("5g");
      expect(form.dockerTmpSizeDirty).toBe(true);
    });
  });
});
