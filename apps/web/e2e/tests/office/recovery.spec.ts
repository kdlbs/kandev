import { test, expect } from "../../fixtures/office-fixture";

test("recovery lookback settings persist and return to 24 hours", async ({
  officeApi,
  officeSeed,
}) => {
  const { settings } = (await officeApi.getWorkspaceSettings(officeSeed.workspaceId)) as {
    settings: { recovery_lookback_hours: number };
  };
  const original = settings.recovery_lookback_hours;
  expect(typeof original).toBe("number");
  expect(original).toBeGreaterThanOrEqual(0);
  try {
    for (const hours of [42, 48, 72, 24]) {
      expect(
        await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
          recovery_lookback_hours: hours,
        }),
      ).toEqual({ ok: true });
      const response = (await officeApi.getWorkspaceSettings(officeSeed.workspaceId)) as {
        settings: { recovery_lookback_hours: number };
      };
      expect(response.settings.recovery_lookback_hours).toBe(hours);
    }
  } finally {
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, {
      recovery_lookback_hours: original,
    });
  }
});
