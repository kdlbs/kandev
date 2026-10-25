import { test, expect } from "../../fixtures/office-fixture";

test("permission settings round-trip human and auto approval modes and reject invalid values", async ({
  officeApi,
  officeSeed,
}) => {
  const initial = (await officeApi.getWorkspaceSettings(officeSeed.workspaceId)) as {
    settings: { name: string; permission_handling_mode: string };
  };
  expect(typeof initial.settings.name).toBe("string");
  expect(typeof initial.settings.permission_handling_mode).toBe("string");
  expect(initial.settings.permission_handling_mode).toBe("human");
  try {
    for (const mode of ["auto_approve", "human"]) {
      await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
        permission_handling_mode: mode,
      });
      const current = (await officeApi.getWorkspaceSettings(officeSeed.workspaceId)) as {
        settings: { permission_handling_mode: string };
      };
      expect(current.settings.permission_handling_mode).toBe(mode);
    }
    await expect(
      officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
        permission_handling_mode: "invalid_mode",
      }),
    ).rejects.toThrow();
  } finally {
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      permission_handling_mode: initial.settings.permission_handling_mode,
    });
  }
});
