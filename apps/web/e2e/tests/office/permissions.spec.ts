import { test, expect } from "../../fixtures/office-fixture";

type WorkspaceSettings = {
  name: string;
  description?: string;
  permission_handling_mode: string;
};

type WorkspaceSettingsResponse = {
  settings: WorkspaceSettings;
};

/**
 * Permission Settings E2E tests.
 *
 * These tests are API-driven and validate that workspace permission settings
 * can be read and updated correctly via the office API.
 */
test.describe("Permission Settings", () => {
  test("default permission handling mode is human", async ({ officeApi, officeSeed }) => {
    const resp = (await officeApi.getWorkspaceSettings(
      officeSeed.workspaceId,
    )) as WorkspaceSettingsResponse;

    expect(resp).toHaveProperty("settings");
    expect(resp.settings.permission_handling_mode).toBe("human");
  });

  test("workspace settings response has required fields", async ({ officeApi, officeSeed }) => {
    const resp = (await officeApi.getWorkspaceSettings(
      officeSeed.workspaceId,
    )) as WorkspaceSettingsResponse;

    expect(resp).toHaveProperty("settings");
    expect(typeof resp.settings.name).toBe("string");
    expect(typeof resp.settings.permission_handling_mode).toBe("string");
  });

  test("can update permission handling mode to auto_approve", async ({ officeApi, officeSeed }) => {
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      permission_handling_mode: "auto_approve",
    });

    const resp = (await officeApi.getWorkspaceSettings(
      officeSeed.workspaceId,
    )) as WorkspaceSettingsResponse;

    expect(resp.settings.permission_handling_mode).toBe("auto_approve");

    // Reset back to human so subsequent tests in this worker start clean.
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      permission_handling_mode: "human",
    });
  });

  test("can update permission handling mode back to human", async ({ officeApi, officeSeed }) => {
    // Set to auto_approve first, then flip back.
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      permission_handling_mode: "auto_approve",
    });
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      permission_handling_mode: "human",
    });

    const resp = (await officeApi.getWorkspaceSettings(
      officeSeed.workspaceId,
    )) as WorkspaceSettingsResponse;

    expect(resp.settings.permission_handling_mode).toBe("human");
  });

  test("invalid permission handling mode is rejected", async ({ officeApi, officeSeed }) => {
    await expect(
      officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
        permission_handling_mode: "invalid_mode",
      }),
    ).rejects.toThrow();
  });
});
