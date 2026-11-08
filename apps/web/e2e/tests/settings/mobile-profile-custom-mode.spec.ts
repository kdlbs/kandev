import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";
import { installProfileRuntimeObservationFixture } from "../../helpers/profile-capability-discovery";

const STORED_MODE = "reviewer";
const REPLACEMENT_MODE = "auditor";
const PROBE_PATH = /^\/api\/v1\/agent-models\/mock-agent\/probe$/;
const RESOLVE_PATH = /\/api\/v1\/agent-models\/mock-agent\/resolve$/;

function resolvedMode(page: Page, mode: string) {
  return waitForHttp(page, "POST", RESOLVE_PATH, {
    predicate: (response) => {
      const request = response.request().postDataJSON() as { mode?: string };
      return response.status() === 200 && request.mode === mode;
    },
  });
}

test.describe("Mobile profile custom modes", () => {
  test("replaces and persists a stored mode when discovery returns an empty list", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");

    const profile = await apiClient.createAgentProfile(agent.id, "Mobile empty mode catalog", {
      model: "mock-fast",
      mode: STORED_MODE,
    });
    await installProfileRuntimeObservationFixture(testPage);

    try {
      const capabilitiesLoaded = waitForHttp(testPage, "POST", PROBE_PATH);
      const storedModeResolved = resolvedMode(testPage, STORED_MODE);
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await Promise.all([capabilitiesLoaded, storedModeResolved]);
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
      );

      const modeField = testPage.getByTestId("profile-mode-field");
      const trigger = testPage.getByTestId("profile-mode-select");
      await expect(modeField).toBeVisible();
      await expect(trigger).toHaveCount(1);
      await expect(trigger).toContainText(STORED_MODE);
      const triggerBounds = await trigger.boundingBox();
      expect(triggerBounds).not.toBeNull();
      expect(triggerBounds!.height).toBeGreaterThanOrEqual(44);
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);

      await trigger.tap();
      const storedRow = testPage.getByTestId("profile-mode-custom-row");
      await expect(storedRow).toHaveCount(1);
      await expect(storedRow).toBeVisible();
      await expect(storedRow).toContainText(STORED_MODE);

      await testPage.getByPlaceholder("Search modes...").fill(REPLACEMENT_MODE);
      const customEntry = testPage.getByTestId("profile-mode-custom-entry");
      await expect(customEntry).toHaveCount(1);
      await expect(customEntry).toBeVisible();
      const replacementResolvedAfterSelection = resolvedMode(testPage, REPLACEMENT_MODE);
      await customEntry.tap();
      await replacementResolvedAfterSelection;
      await expect(trigger).toContainText(REPLACEMENT_MODE);

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toHaveCount(1);
      await expect(saveButton).toBeEnabled();
      const profileSaved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/agent-profiles/${profile.id}$`),
        { predicate: (response) => response.status() === 200 },
      );
      await saveButton.tap();
      await profileSaved;
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden();
      expect((await apiClient.getAgentProfile(profile.id)).mode).toBe(REPLACEMENT_MODE);

      const capabilitiesReloaded = waitForHttp(testPage, "POST", PROBE_PATH);
      const replacementResolved = resolvedMode(testPage, REPLACEMENT_MODE);
      await testPage.reload();
      await Promise.all([capabilitiesReloaded, replacementResolved]);
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
      );
      await expect(testPage.getByTestId("profile-mode-field")).toBeVisible();
      const reloadedTrigger = testPage.getByTestId("profile-mode-select");
      await expect(reloadedTrigger).toContainText(REPLACEMENT_MODE);
      await reloadedTrigger.tap();
      await expect(testPage.getByTestId("profile-mode-custom-row")).toContainText(REPLACEMENT_MODE);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
