import { test, expect } from "../../fixtures/test-base";

test.describe("Agent profile deletion", () => {
  test("deleting profile with no active sessions shows confirm dialog then succeeds", async ({
    testPage,
    apiClient,
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
    await testPage.getByRole("button", { name: "Delete", exact: true }).click();

    // Confirmation dialog should appear
    const dialog = testPage.getByRole("alertdialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByText("This action cannot be undone")).toBeVisible();

    // Confirm the deletion
    await dialog.getByRole("button", { name: "Delete", exact: true }).click();

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
    await testPage.getByRole("button", { name: "Delete", exact: true }).click();

    // Initial confirmation dialog should appear
    const confirmDialog = testPage.getByRole("alertdialog");
    await expect(confirmDialog).toBeVisible({ timeout: 10_000 });

    // Confirm the initial deletion
    await confirmDialog.getByRole("button", { name: "Delete", exact: true }).click();

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
    await testPage.getByRole("button", { name: "Delete", exact: true }).click();

    // Initial confirmation dialog should appear
    const confirmDialog = testPage.getByRole("alertdialog");
    await expect(confirmDialog).toBeVisible({ timeout: 10_000 });

    // Confirm the initial deletion
    await confirmDialog.getByRole("button", { name: "Delete", exact: true }).click();

    // Conflict dialog should appear with active session info
    const conflictDialog = testPage.getByRole("alertdialog");
    await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
    await expect(conflictDialog.getByText("Task For Force Delete")).toBeVisible();

    // Confirm force deletion
    await conflictDialog.getByRole("button", { name: "Delete Anyway" }).click();

    // Should redirect to agents settings page
    await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });
  });
});
