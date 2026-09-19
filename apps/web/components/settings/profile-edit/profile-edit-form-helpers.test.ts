import { describe, expect, it } from "vitest";
import { buildProfileEnvVars } from "./profile-edit-form-helpers";

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
