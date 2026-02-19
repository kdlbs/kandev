import { describe, expect, it } from "vitest";
import {
  profileToPermissionsMap,
  permissionsToProfilePatch,
  buildDefaultPermissions,
  arePermissionsDirty,
} from "./agent-permissions";
import type { PermissionSetting } from "@/lib/types/http";

const mockSettings: Record<string, PermissionSetting> = {
  auto_approve: { supported: true, default: true, label: "Auto approve", description: "" },
  dangerously_skip_permissions: {
    supported: true,
    default: false,
    label: "Skip perms",
    description: "",
  },
  allow_indexing: { supported: true, default: true, label: "Indexing", description: "" },
};

describe("profileToPermissionsMap", () => {
  it("uses profile values when present", () => {
    const result = profileToPermissionsMap(
      { auto_approve: false, dangerously_skip_permissions: true, allow_indexing: false },
      mockSettings,
    );
    expect(result).toEqual({
      auto_approve: false,
      dangerously_skip_permissions: true,
      allow_indexing: false,
    });
  });

  it("falls back to setting defaults for missing keys", () => {
    const result = profileToPermissionsMap({}, mockSettings);
    expect(result).toEqual({
      auto_approve: true,
      dangerously_skip_permissions: false,
      allow_indexing: true,
    });
  });

  it("falls back to false when setting is also missing", () => {
    const result = profileToPermissionsMap({}, {});
    expect(result).toEqual({
      auto_approve: false,
      dangerously_skip_permissions: false,
      allow_indexing: false,
    });
  });
});

describe("permissionsToProfilePatch", () => {
  it("returns all permission keys with values", () => {
    const result = permissionsToProfilePatch({
      auto_approve: true,
      dangerously_skip_permissions: true,
      allow_indexing: true,
    });
    expect(result).toEqual({
      auto_approve: true,
      dangerously_skip_permissions: true,
      allow_indexing: true,
    });
  });

  it("defaults missing keys to false", () => {
    const result = permissionsToProfilePatch({});
    expect(result).toEqual({
      auto_approve: false,
      dangerously_skip_permissions: false,
      allow_indexing: false,
    });
  });
});

describe("buildDefaultPermissions", () => {
  it("builds defaults from permission settings", () => {
    const result = buildDefaultPermissions(mockSettings);
    expect(result).toEqual({
      auto_approve: true,
      dangerously_skip_permissions: false,
      allow_indexing: true,
    });
  });

  it("defaults to false for missing settings", () => {
    const result = buildDefaultPermissions({});
    expect(result).toEqual({
      auto_approve: false,
      dangerously_skip_permissions: false,
      allow_indexing: false,
    });
  });
});

describe("arePermissionsDirty", () => {
  it("returns false when all permissions match", () => {
    const a = { auto_approve: true, dangerously_skip_permissions: false, allow_indexing: true };
    expect(arePermissionsDirty(a, { ...a })).toBe(false);
  });

  it("returns true when any permission differs", () => {
    const a = { auto_approve: true, dangerously_skip_permissions: false, allow_indexing: true };
    const b = { auto_approve: true, dangerously_skip_permissions: true, allow_indexing: true };
    expect(arePermissionsDirty(a, b)).toBe(true);
  });

  it("detects difference when one side has undefined", () => {
    const a = { auto_approve: true };
    const b = { auto_approve: false };
    expect(arePermissionsDirty(a, b)).toBe(true);
  });
});
