import { describe, expect, it } from "vitest";

import { buildProfileConfig } from "./build-create-profile-config";

describe("buildProfileConfig", () => {
  it("includes Cursor Cloud credentials in the create-profile config", () => {
    const config = buildProfileConfig({
      isCursorCloud: true,
      cursorCloudSecretId: "secret-1",
      cursorCloudCallbackUrl: "https://kandev.example/api/v1/managed-agent-mcp",
      isRemote: false,
      isSprites: false,
      isDocker: false,
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
      primaryNetwork: "",
      primaryGwPriority: "",
      additionalNetworks: [],
    });

    expect(config).toEqual({
      cursor_cloud_api_key_secret_id: "secret-1",
      cursor_cloud_callback_url: "https://kandev.example/api/v1/managed-agent-mcp",
    });
  });
});
