import { test, expect } from "../../fixtures/test-base";
import { seededAgentProfile } from "../../helpers/seeded-agent-profile";

test.describe("Agent settings profile layout on mobile", () => {
  test("keeps creation reachable without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // The seeded profile's owner is stable; list order is not, and a virtual
    // family such as Dynamic can sort first with no profiles of its own.
    const { agent } = await seededAgentProfile(apiClient, seedData.agentProfileId);

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
    seedData,
  }) => {
    const { agent, profile: seedProfile } = await seededAgentProfile(
      apiClient,
      seedData.agentProfileId,
    );

    const fallbackModel = `saved-explicit-model-${"x".repeat(128)}`;
    const profileName = "Mobile fallback summary";
    let profileId: string | undefined;

    try {
      await testPage.goto("/settings/agents");
      const seededRow = testPage
        .getByTestId("agent-profile-row")
        .filter({ hasText: seedProfile.name });
      await expect(seededRow).toBeVisible({ timeout: 15_000 });

      const profile = await apiClient.createAgentProfile(agent.id, profileName, {
        model: seedProfile.model,
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
      await expect(fallbackBadge).toBeVisible();
      await expect(async () => {
        const geometry = await fallbackBadge.evaluate((element) => {
          const owner = element.closest('[data-testid="agent-profile-row"]');
          if (!owner) throw new Error("Fallback badge has no profile row");
          return {
            badge: element.getBoundingClientRect().toJSON(),
            row: owner.getBoundingClientRect().toJSON(),
            fitsWidth: element.scrollWidth <= element.clientWidth + 1,
            fitsHeight: element.scrollHeight <= element.clientHeight + 1,
          };
        });
        expect(geometry.badge.width).toBeGreaterThan(0);
        expect(geometry.badge.height).toBeGreaterThan(0);
        expect(geometry.badge.width).toBeLessThanOrEqual(geometry.row.width);
        expect(geometry.fitsWidth).toBe(true);
        expect(geometry.fitsHeight).toBe(true);
        expect(geometry.badge.y).toBeGreaterThanOrEqual(geometry.row.y - 1);
        expect(geometry.badge.y + geometry.badge.height).toBeLessThanOrEqual(
          geometry.row.y + geometry.row.height + 1,
        );
      }).toPass({ timeout: 5_000 });
    } finally {
      if (profileId) {
        await apiClient.deleteAgentProfile(profileId, true);
      }
    }
  });
});
