import { describe, expect, it } from "vitest";
import { buildProfileConfig } from "./create-profile-config";

describe("buildProfileConfig", () => {
  it("preserves Docker network settings alongside Cursor Cloud config", () => {
    const config = buildProfileConfig({
      isCursorCloud: true,
      cursorCloudSecretId: "cursor-key",
      cursorCloudCallbackUrl: "https://kandev.example/api/v1/managed-agent-mcp",
      isRemote: false,
      isSprites: false,
      isDocker: true,
      isLocalDocker: false,
      networkPolicyRules: [],
      remoteCredentials: [],
      configBundleIds: [],
      agentEnvVars: {},
      gitIdentityMode: "override",
      localGitIdentity: { userName: "", userEmail: "", detected: false },
      gitUserName: "",
      gitUserEmail: "",
      dockerfile: "",
      imageTag: "",
      allowUserNamespaces: false,
      primaryNetwork: "shared",
      primaryGwPriority: "100",
      additionalNetworks: [{ name: "database", gwPriority: "20" }],
    });

    expect(config).toEqual({
      docker_network: "shared",
      docker_network_gw_priority: "100",
      docker_additional_networks: JSON.stringify([{ name: "database", gw_priority: 20 }]),
      cursor_cloud_api_key_secret_id: "cursor-key",
      cursor_cloud_callback_url: "https://kandev.example/api/v1/managed-agent-mcp",
    });
  });
});
