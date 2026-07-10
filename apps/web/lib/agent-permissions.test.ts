import { describe, it, expect } from "vitest";
import {
  arePermissionsDirty,
  permissionsToProfilePatch,
  profilePermissionValues,
  readPermissionValue,
} from "./agent-permissions";

describe("readPermissionValue", () => {
  it("prefers camelCase over stale snake_case defaults", () => {
    expect(
      readPermissionValue({ auto_approve: false, autoApprove: true }, "auto_approve", {}),
    ).toBe(true);
  });

  it("falls back to snake_case when camelCase is absent", () => {
    expect(readPermissionValue({ auto_approve: true }, "auto_approve", {})).toBe(true);
  });
});

describe("permissionsToProfilePatch", () => {
  it("reads camelCase draft fields for API payload", () => {
    expect(permissionsToProfilePatch({ autoApprove: true, allowIndexing: false })).toEqual({
      auto_approve: true,
      allow_indexing: false,
    });
  });
});

describe("arePermissionsDirty", () => {
  it("detects camelCase permission edits", () => {
    expect(arePermissionsDirty({ autoApprove: true }, { autoApprove: false })).toBe(true);
  });
});

describe("profilePermissionValues", () => {
  it("normalizes mixed shapes to snake_case", () => {
    expect(profilePermissionValues({ autoApprove: true, allow_indexing: false })).toEqual({
      auto_approve: true,
      allow_indexing: false,
    });
  });
});
