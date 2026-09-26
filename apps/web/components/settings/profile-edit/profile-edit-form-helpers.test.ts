import { describe, expect, it } from "vitest";
import { buildProfileEnvVars, profileSaveInvalidReason } from "./profile-edit-form-helpers";

describe("buildProfileEnvVars", () => {
  it("preserves SPRITES_API_TOKEN on non-Sprites profiles", () => {
    expect(
      buildProfileEnvVars(
        [
          {
            key: "SPRITES_API_TOKEN",
            mode: "secret",
            value: "",
            secretId: "existing-secret",
          },
        ],
        false,
        null,
      ),
    ).toEqual([{ key: "SPRITES_API_TOKEN", secret_id: "existing-secret" }]);
  });
});

describe("profileSaveInvalidReason", () => {
  const t = (key: string) => key;
  const base = {
    isKubernetes: false,
    name: "Docker",
    mcpPolicyErrorKey: null,
    isSprites: false,
    spritesSecretId: null,
    kubernetesProfile: {} as never,
    isDocker: true,
    primaryNetwork: "kandev",
    primaryGwPriority: "",
    additionalNetworks: [],
  };

  it("blocks a save whose gateway priority the backend cannot read", () => {
    expect(profileSaveInvalidReason({ ...base, primaryGwPriority: "1.5" }, false, t)).toBe(
      "executors:gatewayPriorityMustBeAWholeNumber",
    );
  });

  it("allows a whole-number gateway priority", () => {
    expect(
      profileSaveInvalidReason({ ...base, primaryGwPriority: "10" }, false, t),
    ).toBeUndefined();
  });
});
