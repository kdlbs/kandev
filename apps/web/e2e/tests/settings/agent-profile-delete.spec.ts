import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

async function openInitialDeleteConfirmation(page: Page) {
  await page.getByTestId("profile-delete-trigger").click();
  const confirmation = page.getByTestId("agent-profile-delete-confirm-popover");
  await expect(confirmation).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  return confirmation;
}

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

test.describe("Agent profile deletion", () => {
  test("deleting profile with no active sessions shows confirm dialog then succeeds", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(60_000);

    // Create a test profile to delete
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Delete Me", {
      model: agent.profiles[0].model,
    });

    // Navigate to profile settings page
    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

    // Wait for the delete card to load (the card title is "Delete profile")
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // Click the delete button inside the delete card
    const confirmation = await openInitialDeleteConfirmation(testPage);
    await expect(
      confirmation.getByText("This will permanently delete this profile."),
    ).toBeVisible();
    await prCapture.screenshot("desktop-agent-profile-delete-confirmation", {
      caption: "Desktop agent profile deletion confirmation",
    });

    // Confirm the deletion
    await confirmation.getByTestId("agent-profile-delete-confirm").click();

    // Should redirect to agents settings page
    await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });
  });

  test("deleting profile with active task shows confirm then conflict dialog and allows cancel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Create a test profile
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Busy Profile", {
      model: agent.profiles[0].model,
    });

    // Create a task using this profile (this creates an active session)
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Active Task For Profile",
      profile.id,
      {
        description: 'e2e:message("profile test")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Navigate to profile settings page
    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

    // Wait for the delete card to load (the card title is "Delete profile")
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // Click the delete button
    const confirmDialog = await openInitialDeleteConfirmation(testPage);

    // Confirm the initial deletion
    await confirmDialog.getByTestId("agent-profile-delete-confirm").click();

    // The conflict dialog should appear with active session info
    const conflictDialog = testPage.getByRole("alertdialog");
    await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
    await expect(conflictDialog.getByText("Active Task For Profile")).toBeVisible();
    await expect(conflictDialog.getByText("This profile is currently in use")).toBeVisible();

    // Cancel the deletion
    await conflictDialog.getByRole("button", { name: "Cancel" }).click();

    // Dialog should close and we should still be on the profile page
    await expect(conflictDialog).not.toBeVisible();
    await expect(testPage).toHaveURL(
      new RegExp(`/settings/agents/${agent.name}/profiles/${profile.id}`),
    );
  });

  test("deleting profile referenced by a watcher shows watcher in conflict dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Linear config has to be present before /api/v1/linear/watches/issue
    // accepts the create POST; the auth-health probe is also what surfaces
    // hasSecret on the watch row's downstream consumers.
    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    await apiClient.waitForIntegrationAuthHealthy("linear");

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Watched Profile", {
      model: agent.profiles[0].model,
    });

    await apiClient.createLinearIssueWatch({
      workspaceId: seedData.workspaceId,
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      agentProfileId: profile.id,
      filter: { teamKey: "ENG" },
    });

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    const confirmDialog = await openInitialDeleteConfirmation(testPage);
    await confirmDialog.getByTestId("agent-profile-delete-confirm").click();

    // Conflict dialog pops with NO active sessions — only the watcher path.
    // Without the cycle-5 frontend wiring the dialog would either not pop
    // (sessions-only check) or pop empty (watchers ignored).
    const conflictDialog = testPage.getByRole("alertdialog");
    await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
    await expect(conflictDialog.getByText("Watchers (will be disabled):")).toBeVisible();
    await expect(conflictDialog.getByText(/Linear:/)).toBeVisible();
    await expect(conflictDialog.getByText(/team ENG/)).toBeVisible();

    await conflictDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(conflictDialog).not.toBeVisible();
  });

  test("force-deleting profile with watcher disables the watcher row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    await apiClient.waitForIntegrationAuthHealthy("linear");

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Watched ForceRemove", {
      model: agent.profiles[0].model,
    });

    const watch = await apiClient.createLinearIssueWatch({
      workspaceId: seedData.workspaceId,
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      agentProfileId: profile.id,
      filter: { teamKey: "ENG" },
    });
    expect(watch.enabled).toBe(true);

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    const confirmDialog = await openInitialDeleteConfirmation(testPage);
    await confirmDialog.getByTestId("agent-profile-delete-confirm").click();

    const conflictDialog = testPage.getByRole("alertdialog");
    await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
    await conflictDialog.getByRole("button", { name: "Delete Anyway" }).click();

    await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });

    // The eager-disable path (DeleteProfile disables referencing watchers
    // after the row delete succeeds) must have flipped the watcher row to
    // enabled=0 and stamped a cause — without it the watcher would stay live,
    // orphaned at the now-deleted profile, and only self-heal whenever the
    // next external issue happens to match its filter (could be never for a
    // narrow filter).
    const after = await apiClient.getLinearIssueWatch(seedData.workspaceId, watch.id);
    expect(after).not.toBeNull();
    expect(after!.enabled).toBe(false);
    expect(after!.lastError ?? "").toContain(profile.id);
  });

  test("force-deleting profile with active task succeeds after both confirmations", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Create a test profile
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "ForceRemove Profile", {
      model: agent.profiles[0].model,
    });

    // Create a task using this profile
    await apiClient.createTaskWithAgent(seedData.workspaceId, "Task For Force Delete", profile.id, {
      description: 'e2e:message("force delete test")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    // Navigate to profile settings page
    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

    // Wait for the delete card to load (the card title is "Delete profile")
    await expect(testPage.getByText("Delete profile", { exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // Click the delete button
    const confirmDialog = await openInitialDeleteConfirmation(testPage);

    // Confirm the initial deletion
    await confirmDialog.getByTestId("agent-profile-delete-confirm").click();

    // Conflict dialog should appear with active session info
    const conflictDialog = testPage.getByRole("alertdialog");
    await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
    await expect(conflictDialog.getByText("Task For Force Delete")).toBeVisible();

    // Confirm force deletion
    await conflictDialog.getByRole("button", { name: "Delete Anyway" }).click();

    // Should redirect to agents settings page
    await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });
  });

  test("keeps conflict actions visible with many tasks", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Overflow Profile", {
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

    const confirmDialog = await openInitialDeleteConfirmation(testPage);
    await confirmDialog.getByTestId("agent-profile-delete-confirm").click();

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
    const dialogBox = await conflictDialog.boundingBox();
    const titleBox = await title.boundingBox();
    const footerBox = await footer.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(titleBox).not.toBeNull();
    expect(footerBox).not.toBeNull();
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
    const footerAfterBox = await footer.boundingBox();
    expect(finalTaskBox).not.toBeNull();
    expect(footerAfterBox).not.toBeNull();
    expect(finalTaskBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(finalTaskBox!.y + finalTaskBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );
    expect(footerAfterBox!.y).toBeGreaterThanOrEqual(dialogBox!.y);
    expect(footerAfterBox!.y + footerAfterBox!.height).toBeLessThanOrEqual(
      dialogBox!.y + dialogBox!.height,
    );

    for (const label of ["Cancel", "Delete Anyway"]) {
      const action = conflictDialog.getByRole("button", { name: label, exact: true });
      await expect(action).toBeVisible();
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

    await conflictDialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(conflictDialog).not.toBeVisible();
    await apiClient.deleteAgentProfile(profile.id, true);
  });
});
