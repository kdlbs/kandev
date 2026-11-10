import { describe, expect, it } from "vitest";
import {
  isAutomaticPermissionPresentation,
  isUnavailablePermissionPresentation,
  parsePermissionPresentation,
  permissionPresentationFromMetadata,
  permissionRequestsInput,
} from "./presentation";

describe("permission presentation classification", () => {
  it("keeps the legacy interactive behavior for absent and unknown values", () => {
    expect(parsePermissionPresentation(undefined)).toBe("interactive");
    expect(parsePermissionPresentation(null)).toBe("interactive");
    expect(parsePermissionPresentation("")).toBe("interactive");
    expect(parsePermissionPresentation("future-value")).toBe("interactive");
    expect(permissionPresentationFromMetadata(undefined)).toBe("interactive");
  });

  it("recognizes automatic and unavailable rows", () => {
    expect(parsePermissionPresentation("automatic")).toBe("automatic");
    expect(parsePermissionPresentation("interactive")).toBe("interactive");
    expect(parsePermissionPresentation("unavailable")).toBe("unavailable");
    expect(permissionPresentationFromMetadata({ permission_presentation: "automatic" })).toBe(
      "automatic",
    );
  });

  it("requests input only for interactive rows", () => {
    expect(permissionRequestsInput("interactive")).toBe(true);
    expect(permissionRequestsInput("automatic")).toBe(false);
    expect(permissionRequestsInput("unavailable")).toBe(false);
    expect(isAutomaticPermissionPresentation("automatic")).toBe(true);
    expect(isUnavailablePermissionPresentation("unavailable")).toBe(true);
  });
});
