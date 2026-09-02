import { test, expect } from "../../fixtures/ssh-test-base";
import {
  createKotlinTask,
  openDesktopFile,
  openLspStatus,
  performLspAction,
} from "../lsp/lsp-e2e-helpers";

test.describe("SSH LSP boundary", () => {
  test.describe.configure({ timeout: 180_000 });

  test("explains that language servers are unsupported on SSH tasks", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await createKotlinTask(testPage, apiClient, seedData, backend, {
      title: "SSH Kotlin LSP Boundary",
      executorProfileId: seedData.sshExecutorProfileId,
      repositoryDirectory: "e2e-ssh-repo",
      push: true,
    });
    const remoteFilePath = `Remote-${task.taskId}.kt`;

    // The SSH fixture's file:// origin belongs to the backend host and is not
    // reachable from the remote container. Create the Kotlin file through the
    // real workspace WebSocket so the assertion exercises the SSH task host.
    await task.session.clickTab("Files");
    await testPage.getByRole("button", { name: "New file" }).click();
    const fileNameInput = testPage.getByPlaceholder("filename...");
    await expect(fileNameInput).toBeVisible();
    await fileNameInput.fill(remoteFilePath);
    await fileNameInput.press("Enter");
    await openDesktopFile(testPage, task.session, remoteFilePath);

    const statusButton = testPage.locator('[data-testid="lsp-status-button"]:visible');
    await performLspAction(testPage, "start");
    await expect(statusButton).toHaveAttribute("data-lsp-state", "unsupported", {
      timeout: 15_000,
    });
    await expect(await openLspStatus(testPage)).toContainText(
      /language servers are not supported by this task executor/i,
    );
    await expect(testPage.getByText(/Install kotlin-lsp on the task host/)).toHaveCount(0);
  });
});
