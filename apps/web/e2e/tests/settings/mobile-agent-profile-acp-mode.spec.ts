import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  createProfileModeFixture,
  expectFirstPromptMode,
  resolvesConfigMode,
} from "./profile-acp-mode-helpers";

test.describe("ACP profile config mode on phone", () => {
  test("replaces a saved legacy ID and applies the config value at task start", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const fixture = await createProfileModeFixture(apiClient, "dont_ask");

    try {
      const probeLoaded = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/agent-models/${fixture.agentName}/probe$`),
      );
      const profilePath = `/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`;
      await testPage.goto(profilePath);
      await probeLoaded;
      await expect(testPage.getByTestId("profile-name-input")).toBeVisible();

      const trigger = testPage.getByTestId("profile-mode-select");
      await expect(trigger).toContainText("dont_ask");
      const bounds = await trigger.boundingBox();
      expect(bounds).not.toBeNull();
      expect(bounds!.height).toBeGreaterThanOrEqual(44);
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);

      await trigger.tap();
      const storedRow = testPage.getByTestId("profile-mode-custom-row");
      await expect(storedRow).toBeVisible();
      await expect(storedRow).toContainText("dont_ask");
      if (prCapture.capturing) {
        const popover = testPage.locator('[data-slot="popover-content"]');
        await expect(popover).toBeVisible();
        await waitForFiniteAnimations(popover);
        await prCapture.screenshot("mobile-profile-mode-options");
      }

      const search = testPage.getByPlaceholder("Search modes...");
      await search.fill("Don't Ask");
      const advertisedMode = testPage.getByRole("option").filter({ hasText: /^Don't Ask$/ });
      const resolvedMode = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/agent-models/${fixture.agentName}/resolve$`),
        {
          predicate: resolvesConfigMode("session"),
        },
      );
      await expect(advertisedMode).toBeVisible();
      await advertisedMode.tap();
      await resolvedMode;
      await expect(trigger).toContainText("Don't Ask");

      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i });
      const saved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/agent-profiles/${fixture.profileId}$`),
        { predicate: (response) => response.status() === 200 },
      );
      await save.tap();
      await saved;
      expect((await apiClient.getAgentProfile(fixture.profileId)).mode).toBe("session");

      const reopenedProbe = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/agent-models/${fixture.agentName}/probe$`),
      );
      await testPage.reload();
      await reopenedProbe;
      await expect(testPage.getByTestId("profile-mode-select")).toContainText("Don't Ask");

      await expectFirstPromptMode(testPage, apiClient, seedData, fixture);
    } finally {
      await fixture.cleanup();
    }
  });
});
