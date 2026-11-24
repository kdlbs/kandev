import { test, expect } from "../../fixtures/test-base";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import { expectTouchSquareControl } from "../../helpers/control-sizing";
import path from "node:path";

test.describe.configure({ retries: 0 });

test("copies the complete commit identifier by keyboard and fits desktop and coarse targets", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  const profile = await createStandardProfile(apiClient, "Copy commit desktop profile");
  await apiClient.createTaskWithAgent(seedData.workspaceId, "Copy commit desktop", profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile("copy-commit-desktop.txt", "Synthetic desktop commit\n");
  git.stageAll();
  const sha = git.commit("Synthetic copy identifier desktop");
  const session = await openTaskSession(testPage, "Copy commit desktop");
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.clickTab("Changes");
  await session.expandCommitsSection();
  await testPage.getByTestId(`commit-open-${sha.slice(0, 7)}`).click();
  const detail = testPage.getByTestId("commit-detail-content");
  await expect(detail).toBeVisible();
  const action = detail.getByRole("button", { name: "Copy identifier" });
  await expect(action).toBeVisible();
  const size = await action.boundingBox();
  expect(size!.width).toBeCloseTo(28, 0);
  expect(size!.height).toBeCloseTo(28, 0);
  await testPage.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await action.focus();
  await testPage.keyboard.press("Enter");
  await expect.poll(() => testPage.evaluate(() => navigator.clipboard.readText())).toBe(sha);
  await expect(detail.getByRole("status")).toHaveText("Copied!");
  await prCapture.screenshot("commit-copy-identifier-desktop", {
    caption: "Copy the full commit identifier from the desktop detail",
  });
  const cdp = await testPage.context().newCDPSession(testPage);
  await cdp.send("Emulation.setTouchEmulationEnabled", { enabled: true });
  await expect
    .poll(() => testPage.evaluate(() => matchMedia("(pointer: coarse)").matches))
    .toBe(true);
  await expectTouchSquareControl(action);
  await cdp.detach();
});

test("keeps a touch-sized target on a narrow fine-pointer phone viewport", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  expect(await testPage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
  const profile = await createStandardProfile(apiClient, "Copy commit narrow profile");
  await apiClient.createTaskWithAgent(seedData.workspaceId, "Copy commit narrow", profile.id, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  git.createFile("copy-commit-narrow.txt", "Synthetic narrow commit\n");
  git.stageAll();
  const sha = git.commit("Synthetic copy identifier narrow");
  const session = await openTaskSession(testPage, "Copy commit narrow");
  await session.waitForChatIdle({ timeout: 30_000 });
  await testPage.getByRole("button", { name: "Changes" }).click();
  const toggle = testPage.getByTestId("commits-section-collapse-toggle");
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) !== "true") await toggle.click();
  await testPage.getByTestId(`commit-open-${sha.slice(0, 7)}`).click();
  const action = testPage
    .getByTestId("commit-detail-content")
    .getByRole("button", { name: "Copy identifier" });
  await expect(action).toBeVisible();
  const box = await action.boundingBox();
  await expectTouchSquareControl(action);
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
    true,
  );
});
