import { describe, expect, it } from "vitest";
import {
  buildCursorCloudProfileConfig,
  hasCursorCloudProfileConfiguration,
} from "./cursor-cloud-profile-config";

describe("Cursor Cloud profile configuration", () => {
  it("stores only the secret reference and normalized callback URL", () => {
    expect(
      buildCursorCloudProfileConfig(
        { retained: "value", cursor_cloud_api_key_secret_id: "old" },
        "new-secret",
        " https://kandev.example/api/v1/managed-agent-mcp ",
      ),
    ).toEqual({
      retained: "value",
      cursor_cloud_api_key_secret_id: "new-secret",
      cursor_cloud_callback_url: "https://kandev.example/api/v1/managed-agent-mcp",
    });
  });

  it("removes empty values and rejects incomplete profiles", () => {
    expect(
      buildCursorCloudProfileConfig(
        {
          retained: "value",
          cursor_cloud_api_key_secret_id: "old",
          cursor_cloud_callback_url: "old",
        },
        null,
        "  ",
      ),
    ).toEqual({ retained: "value" });
    expect(hasCursorCloudProfileConfiguration(null, "https://kandev.example/callback")).toBe(false);
    expect(hasCursorCloudProfileConfiguration("secret", " ")).toBe(false);
    expect(hasCursorCloudProfileConfiguration("secret", "https://kandev.example/callback")).toBe(
      true,
    );
  });
});
