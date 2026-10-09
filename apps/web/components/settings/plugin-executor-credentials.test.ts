import { describe, expect, it } from "vitest";
import { pluginCredentialConfig, type PluginCredentialForm } from "./plugin-executor-credentials";

const empty: PluginCredentialForm = {
  remoteCredentials: [],
  configBundleIds: [],
  agentEnvVars: {},
  gitIdentityMode: "override",
  localGitIdentity: { userName: "", userEmail: "", detected: false },
  gitUserName: "",
  gitUserEmail: "",
};

describe("pluginCredentialConfig", () => {
  it("serializes the selection into Kandev-owned profile keys", () => {
    expect(
      pluginCredentialConfig({
        ...empty,
        remoteCredentials: ["agent:opencode-acp:files:0"],
        agentEnvVars: { "agent:claude-acp:env:ANTHROPIC_API_KEY": "secret-1", unset: null },
        gitUserName: " Ada ",
        gitUserEmail: "ada@example.com",
      }),
    ).toEqual({
      remote_credentials: '["agent:opencode-acp:files:0"]',
      agent_config_bundles: "",
      remote_auth_secrets: '{"agent:claude-acp:env:ANTHROPIC_API_KEY":"secret-1"}',
      git_user_name: "Ada",
      git_user_email: "ada@example.com",
    });
  });

  it("sends every key empty when nothing is selected, so the backend clears them", () => {
    expect(Object.values(pluginCredentialConfig(empty))).toEqual(["", "", "", "", ""]);
  });

  it("uses the local git identity in local mode", () => {
    const config = pluginCredentialConfig({
      ...empty,
      gitIdentityMode: "local",
      localGitIdentity: { userName: "Local", userEmail: "local@example.com", detected: true },
    });
    expect(config.git_user_name).toBe("Local");
    expect(config.git_user_email).toBe("local@example.com");
  });
});
