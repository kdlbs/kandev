import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// @covers AC-WORKSPACES-SYMLINK-001.1, AC-WORKSPACES-SYMLINK-001.2, AC-WORKSPACES-SYMLINK-001.4
test("identifies a symlink in Changes and the mobile file viewer", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const repo = path.join(backend.tmpDir, "repos", "e2e-repo");
  const name = "a-long-symbolic-link-filename-for-mobile-containment.txt";
  fs.writeFileSync(path.join(repo, "symlink-target.txt"), "target content\n");
  fs.symlinkSync("symlink-target.txt", path.join(repo, name));
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
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await testPage.getByRole("button", { name: /Changes/ }).tap();
  const row = testPage.getByTestId(`file-row-${name}`);
  await expect(row).toBeVisible({ timeout: 20_000 });
  await expect(row.getByTestId("symlink-indicator")).toBeVisible();
  const edit = row.getByRole("button", { name: "Edit", exact: true });
  expect((await edit.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const rowBounds = (await row.boundingBox())!;
  const markerBounds = (await row.getByTestId("symlink-indicator").boundingBox())!;
  const editBounds = (await edit.boundingBox())!;
  expect(markerBounds.x).toBeGreaterThanOrEqual(rowBounds.x);
  expect(markerBounds.x + markerBounds.width).toBeLessThanOrEqual(editBounds.x);
  await edit.tap();
  const viewer = testPage.getByTestId("mobile-file-viewer-panel");
  await expect(viewer).toBeVisible();
  await expect(viewer.getByTestId("symlink-indicator")).toHaveText("Symlink");
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
  ).toBe(true);
  await viewer.getByRole("button", { name: "Close", exact: true }).tap();
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  await expect(session.fileTreeNode(name).getByTestId("symlink-indicator")).toBeVisible();
  await expect(
    session.fileTreeNode("symlink-target.txt").getByTestId("symlink-indicator"),
  ).toHaveCount(0);
  await session.fileTreeNode("symlink-target.txt").tap();
  await expect(viewer).toBeVisible();
  await expect(viewer.getByTestId("symlink-indicator")).toHaveCount(0);
});
