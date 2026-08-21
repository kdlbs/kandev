import { test, expect } from "../../fixtures/test-base";

test.describe("Agent profile deletion on mobile", () => {
  test("keeps list-row deletion controls above the row link and returns focus on cancel", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];

    await testPage.goto("/settings/agents");

    const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
    await expect(row).toBeVisible({ timeout: 15_000 });
    const trigger = row.getByTestId(`profile-actions-menu-${profile.id}`);
    await trigger.tap();
    await testPage.getByTestId(`delete-profile-${profile.id}`).tap();

    const confirmation = row.getByTestId("agent-profile-delete-inline-confirmation");
    await expect(confirmation).toBeVisible();
    for (const label of ["Cancel", "Delete"]) {
      const action = confirmation.getByRole("button", { name: label, exact: true });
      await expect
        .poll(async () =>
          action.evaluate((element) => {
            const rect = element.getBoundingClientRect();
            const hit = document.elementFromPoint(
              rect.left + rect.width / 2,
              rect.top + rect.height / 2,
            );
            return hit === element || element.contains(hit);
          }),
        )
        .toBe(true);
    }

    await confirmation.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(confirmation).not.toBeVisible();
    await expect(trigger).toBeFocused();
    await expect(testPage).toHaveURL(/\/settings\/agents$/);
  });

  test("keeps simple deletion inline with touch-sized cancel and delete actions", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile Delete Me", {
      model: agent.profiles[0].model,
    });

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    const trigger = testPage.getByTestId("profile-delete-trigger");
    await trigger.tap();

    const confirmation = testPage.getByTestId("agent-profile-delete-inline-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await prCapture.screenshot("mobile-agent-profile-delete-confirmation", {
      caption: "Mobile inline agent profile deletion confirmation",
    });
    await expect
      .poll(async () =>
        testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);

    for (const label of ["Cancel", "Delete"]) {
      const action = confirmation.getByRole("button", { name: label, exact: true });
      await expect(action).toBeVisible();
      await expect
        .poll(async () => {
          const box = await action.boundingBox();
          return box ? Math.min(box.width, box.height) : null;
        })
        .toBeGreaterThanOrEqual(44);
    }

    await confirmation.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(confirmation).not.toBeVisible();
    await expect(trigger).toBeVisible();
    await expect(trigger).toBeFocused();

    await trigger.tap();
    await testPage
      .getByTestId("agent-profile-delete-inline-confirmation")
      .getByTestId("agent-profile-delete-confirm")
      .tap();

    await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });
  });
});
