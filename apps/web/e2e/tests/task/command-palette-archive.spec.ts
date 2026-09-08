import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Archiving the current task from the command palette: Cmd/Ctrl+K, type
 * "archive", Enter, confirm. Mirrors the sidebar archive flow, so the same
 * confirmation dialog and archive-and-switch redirect apply.
 */
test.describe("Command palette archive", () => {
  test("typing 'archive' archives the current task and switches away", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const target = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Palette Archive Target",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Palette Archive Neighbour",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${target.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.taskInSidebar("Palette Archive Target")).toBeVisible({ timeout: 30_000 });

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    const input = dialog.getByRole("combobox");
    await input.fill("archive");
    const option = dialog
      .getByRole("option")
      .filter({ has: testPage.getByText("Archive task", { exact: true }) });
    await expect(option).toBeVisible();
    // Matching commands rank above the task rows a query also fuzzy-matches, so
    // the exact command is what Enter runs without any arrow-key navigation.
    await expect(option).toHaveAttribute("aria-selected", "true");
    await expect(dialog.getByRole("option").first()).toContainText("Archive task");
    await testPage.keyboard.press("Enter");

    // The shared confirmation dialog names the task being archived.
    const confirm = testPage.getByRole("alertdialog");
    await expect(confirm.getByText(/Palette Archive Target/)).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("palette-archive-confirm").click();

    await expect(session.taskInSidebar("Palette Archive Target")).not.toBeVisible({
      timeout: 15_000,
    });
    await expect(session.taskInSidebar("Palette Archive Neighbour")).toBeVisible({
      timeout: 15_000,
    });

    // The archived task is still reachable directly and renders as archived.
    await testPage.goto(`/t/${target.id}`);
    await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible({ timeout: 15_000 });
  });

  test("an archived task offers no archive command", async ({ testPage, apiClient, seedData }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Palette Archive Hidden",
      seedData.agentProfileId,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.archiveTask(task.id);

    await testPage.goto(`/t/${task.id}`);
    // The Unarchive control only renders once the detail view has the archived
    // task, so waiting on it keeps the shortcut press from racing hydration.
    await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible({ timeout: 15_000 });
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 10_000 });

    // Positive control: the other task command is registered, so an empty
    // archive result proves the command is hidden rather than not yet loaded.
    await dialog.getByRole("combobox").fill("subtask");
    await expect(
      dialog
        .getByRole("option")
        .filter({ has: testPage.getByText("Create Subtask", { exact: true }) }),
    ).toBeVisible();

    await dialog.getByRole("combobox").fill("archive");
    await expect(
      dialog
        .getByRole("option")
        .filter({ has: testPage.getByText("Archive task", { exact: true }) }),
    ).toHaveCount(0);
  });
});
