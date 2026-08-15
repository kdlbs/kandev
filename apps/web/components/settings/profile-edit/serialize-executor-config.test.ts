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
