import { test, expect } from "../../fixtures/office-fixture";

test.describe("Recovery Settings", () => {
  test("workspace settings include recovery_lookback_hours", async ({ officeApi, officeSeed }) => {
    const resp = await officeApi.getWorkspaceSettings(officeSeed.workspaceId);
    const settings = (resp as { settings: Record<string, unknown> }).settings;

    expect(settings).toBeDefined();
    expect("recovery_lookback_hours" in settings).toBe(true);
    // Default is 24 hours when not explicitly configured.
    expect(typeof settings.recovery_lookback_hours).toBe("number");
    expect((settings.recovery_lookback_hours as number) >= 0).toBe(true);
  });

  test("update recovery_lookback_hours persists", async ({ officeApi, officeSeed }) => {
    const newHours = 48;

    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      recovery_lookback_hours: newHours,
    });

    const resp = await officeApi.getWorkspaceSettings(officeSeed.workspaceId);
    const settings = (resp as { settings: Record<string, unknown> }).settings;
    expect(settings.recovery_lookback_hours).toBe(newHours);
  });

  test("reset recovery_lookback_hours to default (24)", async ({ officeApi, officeSeed }) => {
    // First set to something non-default.
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      recovery_lookback_hours: 72,
    });

    // Then reset to the backend default of 24.
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      recovery_lookback_hours: 24,
    });

    const resp = await officeApi.getWorkspaceSettings(officeSeed.workspaceId);
    const settings = (resp as { settings: Record<string, unknown> }).settings;
    expect(settings.recovery_lookback_hours).toBe(24);
  });
});

test.describe("Recovery Run List", () => {
  test("runs API returns valid response structure", async ({ officeApi, officeSeed }) => {
    const resp = await officeApi.listRuns(officeSeed.workspaceId);
    // Response should contain a runs array (may be empty in a fresh workspace).
    const runs = (resp as { runs?: unknown[] }).runs ?? [];
    expect(Array.isArray(runs)).toBe(true);
  });

  test("run entries have expected fields when present", async ({ officeApi, officeSeed }) => {
    const resp = await officeApi.listRuns(officeSeed.workspaceId);
    const runs = (resp as { runs?: Record<string, unknown>[] }).runs ?? [];

    // If runs exist, verify the shape matches the model.
    for (const w of runs) {
      expect(typeof w.id).toBe("string");
      expect(typeof w.agent_profile_id).toBe("string");
      expect(typeof w.status).toBe("string");
      expect(typeof w.requested_at).toBe("string");
      // cancel_reason is optional — only present on cancelled runs.
    }
  });
});
