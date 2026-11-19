import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  createProfileModeFixture,
  expectFirstPromptMode,
  resolvesConfigMode,
} from "./profile-acp-mode-helpers";

test.describe("ACP profile config mode on desktop", () => {
  test("saves the config value and applies it before the first task prompt", async ({
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
      await trigger.click();
      const storedMode = testPage.getByTestId("profile-mode-custom-row");
      await expect(storedMode).toContainText("dont_ask");
      if (prCapture.capturing) {
        const popover = testPage.locator('[data-slot="popover-content"]');
        await expect(popover).toBeVisible();
        await waitForFiniteAnimations(popover);
        await prCapture.screenshot("desktop-profile-mode-options");
      }
      const search = testPage.getByPlaceholder("Search modes...");
      await search.fill("Don't Ask");
      const resolvedMode = waitForHttp(
        testPage,
        "POST",
        new RegExp(`/api/v1/agent-models/${fixture.agentName}/resolve$`),
        {
          predicate: resolvesConfigMode("session"),
        },
      );
      await search.press("ArrowDown");
      const advertisedMode = testPage.getByRole("option").filter({ hasText: /^Don't Ask$/ });
      await expect(advertisedMode).toBeVisible();
      await search.press("Enter");
      await resolvedMode;
      await expect(trigger).toContainText("Don't Ask");

      const save = testPage.getByRole("button", { name: /^Save( changes)?$/i });
      const saved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/agent-profiles/${fixture.profileId}$`),
        { predicate: (response) => response.status() === 200 },
      );
      await save.click();
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
