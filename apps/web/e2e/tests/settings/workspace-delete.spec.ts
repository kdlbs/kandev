import { test, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

test.describe("Workspace settings", () => {
  test("creates a Kanban workflow with template steps through workspace settings", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const workspaceName = `Settings Kanban ${Date.now().toString(36)}`;
    const taskTitle = `Kanban Bootstrap Task ${Date.now().toString(36)}`;

    await testPage.goto("/settings/workspace");
    await testPage.getByRole("button", { name: "Add Workspace" }).click();
    await testPage.getByLabel("Workspace Name").fill(workspaceName);
    await testPage.locator("form").getByRole("button", { name: "Add Workspace" }).click();

    // Follow the newly rendered workspace link rather than seeding or creating
    // it through the API: this covers the standard Settings creation flow.
    const workspaceLink = testPage.getByRole("link").filter({
      has: testPage.getByRole("heading", { name: workspaceName, exact: true }),
    });
    await expect(workspaceLink).toBeVisible();
    const workspaceHref = await workspaceLink.getAttribute("href");
    const workspaceId = new URL(workspaceHref ?? "", "http://kandev.test").pathname.split("/")[3];
    expect(workspaceId).toBeTruthy();
    let createdTaskId: string | undefined;

    try {
      await workspaceLink.click();

      await expect(
        testPage.getByRole("heading", { name: workspaceName, exact: true }),
      ).toBeVisible();
      await testPage
        .getByTestId("settings-scroll-container")
        .getByRole("link", { name: "Workflows", exact: true })
        .click();

      const workflows = new WorkflowSettingsPage(testPage);
      const kanbanCard = await workflows.findWorkflowCard("Kanban");
      await expect(kanbanCard).toBeVisible();
      for (const stepName of ["Backlog", "In Progress", "Review", "Done"]) {
        await expect(workflows.stepNodeByName(kanbanCard, stepName)).toBeVisible();
      }
      await prCapture.screenshot("desktop-kanban-workspace-bootstrap", {
        caption:
          "A workspace created from Settings includes the Kanban workflow and its four ready-to-use steps.",
      });

      // Select the new workspace from the user-facing picker, then verify the
      // bootstrap immediately makes its workflow usable by the standard New
      // Task flow. Scratch mode avoids introducing a repository as test setup.
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.getByTestId("sidebar-workspace-trigger").click();
      await testPage.getByTestId(`sidebar-workspace-item-${workspaceId}`).click();
      await expect(testPage).toHaveURL(
        (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === workspaceId,
      );
      await expect(kanban.board).toBeVisible();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await dialog.getByTestId("source-mode-scratch").click();
      await dialog.getByTestId("task-title-input").fill(taskTitle);
      await dialog.getByTestId("task-description-input").fill("Created without starting an agent");
      await expect(dialog.getByTestId("submit-start-agent-chevron")).toBeEnabled();

      const created = testPage.waitForResponse(
        (response) =>
          response.url().endsWith("/api/v1/tasks") && response.request().method() === "POST",
      );
      await dialog.getByTestId("submit-start-agent-chevron").click();
      await testPage.getByTestId("submit-create-without-agent").click();
      const createdTask = (await created).json() as Promise<{ id?: string }>;
      createdTaskId = (await createdTask).id;

      await expect(dialog).not.toBeVisible();
      await expect(testPage).toHaveURL(/\/t\//);
      await expect(testPage.getByRole("link", { name: taskTitle, exact: true })).toBeVisible();
    } finally {
      if (createdTaskId) await apiClient.deleteTask(createdTaskId);
      if (workspaceId) await apiClient.deleteWorkspace(workspaceId, workspaceName);
    }
  });

  test("deletes a workspace from the settings edit page", async ({ testPage, apiClient }) => {
    const suffix = Date.now().toString(36);
    const workspaceName = `Settings Delete ${suffix}`;
    const workspace = await apiClient.createWorkspace(workspaceName);

    await testPage.goto(`/settings/workspace/${workspace.id}`);
    await expect(testPage.getByRole("heading", { name: workspaceName })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("workspace-settings-delete-button").click();

    const confirmInput = testPage.getByTestId("workspace-settings-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-settings-delete-confirm-button");
    await expect(confirmInput).toBeVisible();

    // The wrong confirmation string ("delete") must not enable deletion — the
    // backend requires confirm_name to equal the workspace name.
    await confirmInput.fill("delete");
    await expect(confirmButton).toBeDisabled();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();

    // Cancel-then-reopen must reset the confirmation field; otherwise the
    // re-type requirement would be silently bypassed on the second open.
    await testPage.getByRole("button", { name: "Cancel" }).click();
    await testPage.getByTestId("workspace-settings-delete-button").click();
    await expect(confirmInput).toHaveValue("");
    await expect(confirmButton).toBeDisabled();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();

    // Deletion runs through an action wrapper, so assert the user-visible
    // outcome: redirect to the workspace list and the workspace gone from the
    // backend.
    await confirmButton.click();
    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === workspace.id)).toBe(false);
  });

  test("deletes an office workspace from the settings edit page", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    const officeApi = new OfficeApiClient(backend.baseUrl);
    const suffix = Date.now().toString(36);
    const workspaceName = `Settings Office Delete ${suffix}`;
    const onboarded = await officeApi.completeOnboarding({
      workspaceName,
      taskPrefix: `SOD${suffix.toUpperCase().slice(0, 3)}`,
      agentName: "Settings Delete CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
    });

    await officeApi.createSkill(onboarded.workspaceId, {
      name: `Settings Cleanup Skill ${suffix}`,
      slug: `settings-cleanup-skill-${suffix}`,
      content: "# Settings cleanup skill\n",
    });

    await testPage.goto(`/settings/workspace/${onboarded.workspaceId}`);
    await expect(testPage.getByRole("heading", { name: workspaceName })).toBeVisible({
      timeout: 15_000,
    });

    await testPage.getByTestId("workspace-settings-delete-button").click();
    const confirmInput = testPage.getByTestId("workspace-settings-delete-confirm-input");
    const confirmButton = testPage.getByTestId("workspace-settings-delete-confirm-button");
    await expect(confirmInput).toBeVisible();

    await confirmInput.fill(workspaceName);
    await expect(confirmButton).toBeEnabled();
    await confirmButton.click();
    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });

    const { workspaces } = await apiClient.listWorkspaces();
    expect(workspaces.some((item) => item.id === onboarded.workspaceId)).toBe(false);

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: seedData.workflowId,
      keyboard_shortcuts: {},
      enable_preview_on_click: false,
      sidebar_views: [],
    });
  });
});
