import { test, expect } from "../../fixtures/test-base";

test.describe("Agent settings profile layout on mobile", () => {
  test("keeps creation reachable without horizontal overflow", async ({ testPage, apiClient }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent || agent.profiles.length === 0) {
      throw new Error("The E2E fixture must provide a configured agent profile");
    }

    await testPage.goto("/settings/agents");

    await expect
      .poll(
        async () =>
          testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        { timeout: 15_000 },
      )
      .toBe(true);

    const card = testPage.getByTestId(`agent-group-${agent.name}`);
    const newProfile = card
      .getByTestId(`agent-card-header-${agent.name}`)
      .getByTestId(`new-profile-${agent.name}`);
    await expect(newProfile).toBeVisible({ timeout: 15_000 });
    await expect
      .poll(
        async () => {
          const box = await newProfile.boundingBox();
          return box ? Math.min(box.width, box.height) : null;
        },
        { timeout: 10_000 },
      )
      .toBeGreaterThanOrEqual(44);

    const actions = testPage.getByTestId("installed-agents-actions");
    for (const testId of ["open-host-shell", "rescan-agents-button", "new-agent-button"]) {
      const control = actions.getByTestId(testId);
      await expect(control).toBeVisible();
      await expect
        .poll(
          async () => {
            const box = await control.boundingBox();
            return box ? Math.min(box.width, box.height) : null;
          },
          { timeout: 10_000 },
        )
        .toBeGreaterThanOrEqual(44);
    }

    await newProfile.tap();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/agents/${encodeURIComponent(agent.name)}\\?mode=create$`),
    );
  });

  test("wraps the saved fallback summary without horizontal overflow", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent || agent.profiles.length === 0) {
      throw new Error("The E2E fixture must provide a configured agent profile");
    }

    const fallbackModel = `saved-explicit-model-${"x".repeat(128)}`;
    const profileName = "Mobile fallback summary";
    let profileId: string | undefined;

    try {
      await testPage.goto("/settings/agents");
      const seededRow = testPage
        .getByTestId("agent-profile-row")
        .filter({ hasText: agent.profiles[0].name });
      await expect(seededRow).toBeVisible({ timeout: 15_000 });

      const profile = await apiClient.createAgentProfile(agent.id, profileName, {
        model: agent.profiles[0].model,
        fallback_model: fallbackModel,
      });
      profileId = profile.id;
      await testPage.reload();

      const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () =>
            testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
          { timeout: 15_000 },
        )
        .toBe(true);
      await expect(row.locator('[data-slot="badge"]')).toHaveText([
        profile.model,
        `fallback: ${fallbackModel}`,
      ]);
      const fallbackBadge = row.locator('[data-slot="badge"]').nth(1);
      const [badgeBox, rowBox] = await Promise.all([
        fallbackBadge.boundingBox(),
        row.boundingBox(),
      ]);
      expect(badgeBox).not.toBeNull();
      expect(rowBox).not.toBeNull();
      expect(badgeBox!.width).toBeLessThanOrEqual(rowBox!.width);
      expect(
        await fallbackBadge.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
      ).toBe(true);
      expect(
        await fallbackBadge.evaluate((element) => element.scrollHeight <= element.clientHeight + 1),
      ).toBe(true);
      expect(badgeBox!.y).toBeGreaterThanOrEqual(rowBox!.y - 1);
      expect(badgeBox!.y + badgeBox!.height).toBeLessThanOrEqual(rowBox!.y + rowBox!.height + 1);
    } finally {
      if (profileId) {
        await apiClient.deleteAgentProfile(profileId, true);
      }
    }
  });
});
