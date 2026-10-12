import { describe, expect, it } from "vitest";
import { createDiskUsageQueryKey, type SystemInfoQueryIdentity } from "./system-info-query";

type DiskUsageQueryIdentity = SystemInfoQueryIdentity;

const IDENTITY: DiskUsageQueryIdentity = {
  apiBaseUrl: "https://backend.example/api///",
  bootId: "boot-1",
  authMode: "enabled",
  authenticated: true,
  userId: "user-1",
};

describe("disk usage query identity", () => {
  it("uses the shared normalized backend, boot, and auth identity", () => {
    expect(createDiskUsageQueryKey(IDENTITY)).toEqual([
      "system",
      "disk-usage",
      "https://backend.example/api",
      "boot-1",
      "enabled",
      true,
      "user-1",
    ]);
  });

  it.each([
    [{ ...IDENTITY, apiBaseUrl: "https://other.example/api" }],
    [{ ...IDENTITY, bootId: "boot-2" }],
    [{ ...IDENTITY, authMode: "disabled" as const }],
    [{ ...IDENTITY, authenticated: false, userId: null }],
    [{ ...IDENTITY, userId: "user-2" }],
  ])("isolates a changed identity field: %o", (changedIdentity) => {
    expect(createDiskUsageQueryKey(changedIdentity)).not.toEqual(createDiskUsageQueryKey(IDENTITY));
  });
});
