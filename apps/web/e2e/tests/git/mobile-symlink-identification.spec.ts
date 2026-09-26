import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForLatestSessionDone } from "../../helpers/session";

// @covers AC-WORKSPACES-SYMLINK-001.1, AC-WORKSPACES-SYMLINK-001.2, AC-WORKSPACES-SYMLINK-001.4
test("identifies a symlink in Changes and the mobile file viewer", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  const name = "a-long-symbolic-link-filename-for-mobile-containment.txt";
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile symlink identity",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await waitForLatestSessionDone(
    apiClient,
    task.id,
    1,
    "mobile symlink task did not finish preparing its workspace",
  );
  let workspacePath = "";
  await expect
    .poll(
      async () => {
        const environment = await apiClient.getTaskEnvironment(task.id);
        workspacePath = environment?.repos?.[0]?.worktree_path ?? environment?.workspace_path ?? "";
        return environment?.status === "ready" && workspacePath !== "";
      },
      {
        timeout: 45_000,
        message: "mobile symlink task workspace is ready",
      },
    )
    .toBe(true);

  const targetName = `mobile-symlink-target-${task.id}.txt`;
  const targetPath = path.join(workspacePath, targetName);
  const symlinkPath = path.join(workspacePath, name);

  try {
    fs.rmSync(symlinkPath, { force: true });
    fs.writeFileSync(targetPath, "target content\n");
    fs.symlinkSync(targetName, symlinkPath);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await testPage.getByRole("button", { name: /Changes/ }).tap();
    const row = testPage.getByTestId(`file-row-${name}`);
    await expect(row).toBeVisible({ timeout: 20_000 });
    await expect(row.getByTestId("symlink-indicator")).toBeVisible();
    const actions = row.getByRole("button", { name: "Show more actions", exact: true });
    const actionsBounds = (await actions.boundingBox())!;
    expect(actionsBounds.height).toBeGreaterThanOrEqual(44);
    expect(actionsBounds.width).toBeGreaterThanOrEqual(44);
    const rowBounds = (await row.boundingBox())!;
    const markerBounds = (await row.getByTestId("symlink-indicator").boundingBox())!;
    expect(markerBounds.x).toBeGreaterThanOrEqual(rowBounds.x);
    expect(markerBounds.x + markerBounds.width).toBeLessThanOrEqual(actionsBounds.x);
    await actions.tap();
    const edit = testPage.getByRole("menuitem", { name: "Edit", exact: true });
    await expect(edit).toBeVisible();
    // The menu item has a fixed 44px touch target. Check its layout contract
    // while the transient menu is open, then tap immediately.
    await expect(edit).toHaveCSS("min-height", "44px");
    await edit.tap({ timeout: 5_000 });
    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible();
    await expect(viewer.getByTestId("symlink-indicator")).toHaveText("Symlink");
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    await viewer.getByRole("button", { name: "Close", exact: true }).tap();
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await expect(session.fileTreeNode(name).getByTestId("symlink-indicator")).toBeVisible();
    await expect(session.fileTreeNode(targetName).getByTestId("symlink-indicator")).toHaveCount(0);
    await session.fileTreeNode(targetName).tap();
    await expect(viewer).toBeVisible();
    await expect(viewer.getByTestId("symlink-indicator")).toHaveCount(0);
  } finally {
    fs.rmSync(symlinkPath, { force: true });
    fs.rmSync(targetPath, { force: true });
  }
});
