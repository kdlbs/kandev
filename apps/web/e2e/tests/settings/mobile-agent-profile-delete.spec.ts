import { test, expect } from "../../fixtures/test-base";

function longProfileDeleteConflict() {
  return {
    active_sessions: Array.from({ length: 40 }, (_, index) => ({
      task_id: `overflow-task-${index}`,
      task_title: `Overflow task ${index}`,
      is_ephemeral: false,
    })),
    watchers: [],
    routing_tiers: [],
    automations: [],
    utility_agents: [],
  };
}

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

  test("keeps conflict actions visible with many tasks", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile Overflow Profile", {
      model: agent.profiles[0].model,
    });
    await testPage.route(`**/api/v1/agent-profiles/${profile.id}**`, async (route) => {
      if (route.request().method() !== "DELETE") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 409,
        contentType: "application/json",
        body: JSON.stringify(longProfileDeleteConflict()),
      });
    });

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("profile-delete-trigger").tap();
    const confirmation = testPage.getByTestId("agent-profile-delete-inline-confirmation");
    await expect(confirmation).toBeVisible();
    await confirmation.getByTestId("agent-profile-delete-confirm").tap();

    const conflictDialog = testPage.getByTestId("agent-profile-delete-conflict-dialog");
    await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
    await conflictDialog.evaluate(async (element) => {
      const animations = element.getAnimations({ subtree: true }).filter((animation) => {
        if (animation.playState !== "running") {
          return false;
        }

        const iterations = animation.effect?.getComputedTiming().iterations;
        return typeof iterations === "number" && Number.isFinite(iterations);
      });

      await Promise.all(animations.map((animation) => animation.finished.catch(() => undefined)));
    });
    const body = conflictDialog.getByTestId("agent-profile-delete-conflict-body");
    const footer = conflictDialog.getByTestId("agent-profile-delete-conflict-footer");
    const title = conflictDialog.locator('[data-slot="alert-dialog-title"]');
    const finalTask = body.getByText("Overflow task 39", { exact: true });

    const scrollable = await body.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(scrollable.scrollHeight).toBeGreaterThan(scrollable.clientHeight);

    const viewportHeight = await testPage.evaluate(() => window.innerHeight);
    const viewportWidth = await testPage.evaluate(() => window.innerWidth);
    const dialogBox = await conflictDialog.boundingBox();
    const titleBox = await title.boundingBox();
    const footerBox = await footer.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(titleBox).not.toBeNull();
    expect(footerBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewportWidth);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewportHeight);
    expect(titleBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(titleBox!.y + titleBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);
    expect(footerBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(footerBox!.y + footerBox!.height).toBeLessThanOrEqual(dialogBox!.y + dialogBox!.height);

    await body.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);

    const finalTaskBox = await finalTask.boundingBox();
    expect(finalTaskBox).not.toBeNull();
    expect(finalTaskBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(finalTaskBox!.y + finalTaskBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );
    const footerAfterBox = await footer.boundingBox();
    expect(footerAfterBox).not.toBeNull();
    expect(footerAfterBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(footerAfterBox!.y + footerAfterBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );

    for (const label of ["Cancel", "Delete Anyway"]) {
      const action = conflictDialog.getByRole("button", { name: label, exact: true });
      const box = await action.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
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

    await conflictDialog.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(conflictDialog).not.toBeVisible();
    const documentOverflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(documentOverflow.scrollWidth).toBeLessThanOrEqual(documentOverflow.clientWidth);
    await apiClient.deleteAgentProfile(profile.id, true);
  });
});
