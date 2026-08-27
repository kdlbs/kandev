import { describe, expect, it } from "vitest";
import { buildSaveConfig, type ExecutorProfileConfigForm } from "./serialize-executor-config";

function form(overrides: Partial<ExecutorProfileConfigForm> = {}): ExecutorProfileConfigForm {
  return {
    isSprites: false,
    networkPolicyRules: [],
    isRemote: true,
    remoteCredentials: [],
    configBundleIds: [],
    agentEnvVars: {},
    gitIdentityMode: "override",
    localGitIdentity: { userName: "", userEmail: "" },
    gitUserName: "",
    gitUserEmail: "",
    isDocker: false,
    dockerfile: "",
    imageTag: "",
    isSSH: false,
    sshShell: "",
    sshReclaimTaskDir: false,
    ...overrides,
  };
}

describe("buildSaveConfig", () => {
  it("persists selected configuration without requiring authentication", () => {
    const config = buildSaveConfig(form({ configBundleIds: ["mock.settings"] }), {
      remote_credentials: "stale",
      keep: "yes",
    });

    expect(config).toEqual({
      agent_config_bundles: '["mock.settings"]',
      keep: "yes",
    });
  });

  it("persists authentication without requiring configuration", () => {
    const config = buildSaveConfig(form({ remoteCredentials: ["codex-auth"] }));

    expect(config).toEqual({ remote_credentials: '["codex-auth"]' });
  });
});

describe("buildSaveConfig ssh_reclaim_task_dir", () => {
  it("writes an explicit false for an SSH profile that leaves reclamation off", () => {
    const config = buildSaveConfig(form({ isSSH: true, sshReclaimTaskDir: false }));

    expect(config.ssh_reclaim_task_dir).toBe("false");
  });

  it("writes the exact string the backend compares when reclamation is on", () => {
    const config = buildSaveConfig(form({ isSSH: true, sshReclaimTaskDir: true }));

    expect(config.ssh_reclaim_task_dir).toBe("true");
  });

  it("never arms reclamation on a non-SSH profile, even from a stale stored value", () => {
    const config = buildSaveConfig(form({ isSSH: false, sshReclaimTaskDir: true }), {
      ssh_reclaim_task_dir: "true",
    });

    expect(config.ssh_reclaim_task_dir).toBeUndefined();
  });
});
